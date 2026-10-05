package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

type EneyidaProvider struct {
	client  *TLSClient
	baseURL string
}

func NewEneyidaProvider(client *TLSClient) *EneyidaProvider {
	return &EneyidaProvider{
		client:  client,
		baseURL: "https://eneyida.tv",
	}
}

func (p *EneyidaProvider) ID() string {
	return "eneyida"
}

func (p *EneyidaProvider) Name() string {
	return "Eneyida"
}

func (p *EneyidaProvider) BaseURL() string {
	return p.baseURL
}

// Describe повертає публічні метадані для каталогу провайдерів.
func (p *EneyidaProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID:                   p.ID(),
		Name:                 p.Name(),
		BaseURL:              p.baseURL,
		ShowOnHome:           true,
		HasFixedStreams:      false,
		ContentTypes:         []string{"movie", "series", "cartoon", "anime"},
		SearchEnabledDefault: true,
	}
}

func (p *EneyidaProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	searchURL := fmt.Sprintf("%s/index.php?do=search&subaction=search&story=%s", p.baseURL, url.QueryEscape(query))
	items, err := p.fetchCatalog(ctx, searchURL, "")
	if err != nil {
		return nil, fmt.Errorf("eneyida search error: %w", err)
	}
	return items, nil
}

func (p *EneyidaProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}
	section := p.getSection(contentType)
	var reqURL string
	if section == "" {
		if page == 1 {
			reqURL = p.baseURL + "/"
		} else {
			reqURL = fmt.Sprintf("%s/page/%d/", p.baseURL, page)
		}
	} else {
		if page == 1 {
			reqURL = fmt.Sprintf("%s/%s/", p.baseURL, section)
		} else {
			reqURL = fmt.Sprintf("%s/%s/page/%d/", p.baseURL, section, page)
		}
	}
	return p.fetchCatalog(ctx, reqURL, section)
}

func (p *EneyidaProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}
	if category == "" {
		return p.GetPopular(ctx, contentType, page)
	}
	encoded := url.PathEscape(category)
	var reqURL string
	if page == 1 {
		reqURL = fmt.Sprintf("%s/%s/", p.baseURL, encoded)
	} else {
		reqURL = fmt.Sprintf("%s/%s/page/%d/", p.baseURL, encoded, page)
	}
	// Розділ з contentType все одно каже про вид контенту — наприклад
	// /series/xfsearch/genre/комедія/ це серіал, а не фільм.
	return p.fetchCatalog(ctx, reqURL, p.getSection(contentType))
}

// getSection повертає слаг розділу eneyida.tv для типу контенту.
//
// Слади перевірені живими запитами (200 OK):
//
//	/films/           фільми
//	/series/          серіали
//	/cartoon/         мультфільми
//	/cartoon-series/  мультсеріали
//	/anime/           аніме
//
// Старі значення «serials» та «multfilmy» давали 404 — саме через це
// розділи серіалів і мультфільмів порожніли.
func (p *EneyidaProvider) getSection(contentType string) string {
	switch contentType {
	case "movie":
		return "films"
	case "series":
		return "series"
	case "cartoon":
		return "cartoon"
	case "anime":
		return "anime"
	default:
		return ""
	}
}

// fetchCatalog розбирає сторінку каталогу.
//
// section передається з getSection, щоб тип картки можна було визначити
// з розділу, а не лише з тексту: на eneyida.tv посилання на матеріал
// кореневі (/10199-salmokdzhi-shepit-vody.html) і жодного маркера не
// містять, тож «film»/«mult» з href не працювали.
func (p *EneyidaProvider) fetchCatalog(ctx context.Context, reqURL, section string) ([]domain.MediaItem, error) {
	html, err := p.client.Get(ctx, reqURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida fetch catalog (%s): %w", reqURL, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse eneyida catalog: %w", err)
	}

	// Initialised non-nil so an empty catalogue serialises as `[]`, not
	// `null`. json.Marshal turns a nil slice into `null`, which every client
	// then has to special-case.
	items := []domain.MediaItem{}
	doc.Find("article.short, .short-story").Each(func(i int, s *goquery.Selection) {
		// .short_title на живому сайті — це САМ anchor
		// (<a class="short_title" id="short_title" href=…>Назва</a>),
		// а не h2 з нащадком. Старий селектор «h2.short_title a,
		// .short_title a» шукав нащадка у самого себе і не находив
		// ЖОДНОЇ картки — каталог був порожнім (0 items).
		linkElem := s.Find("a.short_title, h2.short_title a, .short_title a, a.short_btn").First()
		title := strings.TrimSpace(linkElem.Text())
		href, exists := linkElem.Attr("href")
		if !exists || title == "" {
			return
		}

		imgElem := s.Find(".short_img img, img").First()
		poster, _ := imgElem.Attr("src")
		if poster == "" {
			poster, _ = imgElem.Attr("data-src")
		}
		if poster != "" && !strings.HasPrefix(poster, "http") {
			poster = p.baseURL + poster
		}

		// Рік і оригінальна назва живуть у .short_subtitle:
		//   <div class="short_subtitle"><a href="…/xfsearch/year/2024/">2024</a>
		//     &bull; Dark Matter</div>
		year, origTitle := parseEneyidaSubtitle(s.Find(".short_subtitle").First())
		if year == 0 {
			year = parseYear(s.Text())
		}

		mediaType := classifyEneyidaType(s.Text(), href, section)

		items = append(items, domain.MediaItem{
			ID:            href,
			ProviderID:    p.ID(),
			Title:         title,
			OriginalTitle: origTitle,
			PosterURL:     poster,
			Year:          year,
			URL:           href,
			Type:          mediaType,
		})
	})

	return items, nil
}

// parseEneyidaSubtitle дістає (рік, оригінальна назва) з .short_subtitle.
//
// Рік береться з посилання /xfsearch/year/YYYY/, а оригінальна назва —
// це решта тексту після відкидання року та роздільника «•».
func parseEneyidaSubtitle(subtitle *goquery.Selection) (year int, origTitle string) {
	if subtitle == nil || subtitle.Length() == 0 {
		return 0, ""
	}

	yearLink := subtitle.Find(`a[href*="/year/"]`).First()
	if yearLink.Length() > 0 {
		year = parseYear(yearLink.Text())
		// Копія без лінка року — щоб у назві не лишилося «2024 •».
		rest := subtitle.Clone()
		rest.Find(`a[href*="/year/"]`).Remove()
		origTitle = cleanEneyidaSubtitleText(rest.Text())
	} else {
		origTitle = cleanEneyidaSubtitleText(subtitle.Text())
	}

	if year == 0 {
		year = parseYear(origTitle)
	}
	return year, origTitle
}

// cleanEneyidaSubtitleText прибирає булети та крайні пробіли з тексту
// .short_subtitle, щоб оригінальна назва була «Dark Matter», а не
// «• Dark Matter».
func cleanEneyidaSubtitleText(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.NewReplacer("&bull;", " ", "•", " ", "·", " ", "|", " ").Replace(raw)
	return strings.TrimSpace(strings.Join(strings.Fields(raw), " "))
}

// classifyEneyidaType визначає тип матеріалу картки.
//
// Порядок джерел навмисний:
//
//  1. Текст картки. На eneyida.tv серіал/мультсеріал/аніме мають
//     бейдж .metaBottom.label_quel-camrip «2 сезон 1 серія», а фільми —
//     ні. Це єдиний реальний сигнал у картці.
//  2. Розділ каталогу. Він уточнює вид серіалоподібного: «1 сезон
//     12 серія» однаково виглядає в /series/, /anime/ і
//     /cartoon-series/, а розділ каже точно.
//  3. href — лише фолбек для пошуку й головної, де розділу немає.
//
// Раніше був лише крок 3, і він не спрацьовував: посилання на матеріал
// кореневі (/6782-matrycia.html) і не містять жодного маркера.
func classifyEneyidaType(cardText, href, section string) string {
	// Крок 1: текстовий маркер епізодичності.
	if reEneyidaEpisodic.MatchString(cardText) {
		switch section {
		case "anime":
			return "anime"
		case "cartoon", "cartoon-series":
			return "cartoon"
		case "series":
			return "series"
		}
		return "series"
	}

	// Крок 3: фолбек за href (пошук, головна сторінка).
	return eneyidaTypeFromHref(href)
}

// eneyidaTypeFromHref — фолбек класифікації за URL.
//
// Корінь eneyida.tv тримає вкладку «Жанр» зі ссылкою на сам розділ:
// <a href="https://eneyida.tv/series/">серіал</a>,
// <a href="https://eneyida.tv/anime/">аніме</a>,
// <a href="https://eneyida.tv/cartoon/">мультфільм</a>,
// <a href="https://eneyida.tv/films/">фільм</a>.
// Це єдине місце, де тип зберігається у URL.
func eneyidaTypeFromHref(href string) string {
	lower := strings.ToLower(href)
	switch {
	case strings.Contains(lower, "/anime"):
		return "anime"
	case strings.Contains(lower, "/cartoon-series"):
		return "cartoon"
	case strings.Contains(lower, "/cartoon"), strings.Contains(lower, "/mult"):
		return "cartoon"
	case strings.Contains(lower, "/series"), strings.Contains(lower, "/serial"):
		return "series"
	case strings.Contains(lower, "/films"):
		return "movie"
	}
	return "movie"
}

// reEneyidaEpisodic ловить бейдж «N сезон M серія» у тексті картки.
var reEneyidaEpisodic = regexp.MustCompile(`(?i)(?:сезон[а-я'’їєґ]*|серіал\w*|сері[я'’їєґ]+)`)

func (p *EneyidaProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida get details: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse details html: %w", err)
	}

	title := strings.TrimSpace(doc.Find("h1.full-title, h1").First().Text())
	// .full_header-subtitle — реальний контейнер оригінальної назви на
	// eneyida.tv; старий .full_orig-title на живому сайті не існує.
	origTitle := strings.TrimSpace(doc.Find(".full_header-subtitle, .full_orig-title, [itemprop='alternateName']").First().Text())
	if origTitle == "" {
		origTitle, _ = doc.Find("meta[itemprop='alternateName']").Attr("content")
		origTitle = strings.TrimSpace(origTitle)
	}

	poster, _ := doc.Find(".full_content-poster img, .full-poster img, .full_poster img").Attr("src")
	if poster == "" {
		poster, _ = doc.Find(".full_content-poster img, .full-poster img, .full_poster img").Attr("data-src")
	}
	if poster == "" {
		poster, _ = doc.Find("img[src*='/uploads/posts/']").First().Attr("src")
	}
	if poster != "" && !strings.HasPrefix(poster, "http") {
		poster = p.baseURL + poster
	}
	// .full_content-desc — опис на живому сайті. Порядок навмисний:
	// .full-text у шаблоні eneyida це блок КОМЕНТАРІВ, і без
	// .full_content-desc першим збігом був би коментар під описом.
	desc := strings.TrimSpace(doc.Find(".full_content-desc, .full_text, .full-text").First().Text())

	var genres []string
	var countries []string
	var actors []string
	var director string
	var duration string
	var year int

	// Schema.org metas
	if g, ok := doc.Find("meta[itemprop='genre']").Attr("content"); ok {
		for _, part := range strings.Split(g, ",") {
			if t := strings.TrimSpace(part); t != "" {
				genres = append(genres, t)
			}
		}
	}
	if c, ok := doc.Find("meta[itemprop='contentLocation'], meta[itemprop='countryOfOrigin']").Attr("content"); ok {
		for _, part := range strings.Split(c, ",") {
			if t := strings.TrimSpace(part); t != "" {
				countries = append(countries, t)
			}
		}
	}
	if a, ok := doc.Find("meta[itemprop='actors']").Attr("content"); ok {
		for _, part := range strings.Split(a, ",") {
			if t := strings.TrimSpace(part); t != "" {
				actors = append(actors, t)
			}
		}
	}
	if d, ok := doc.Find("meta[itemprop='director']").Attr("content"); ok {
		director = strings.TrimSpace(d)
	}
	if yrStr, ok := doc.Find("meta[itemprop='dateCreated']").Attr("content"); ok {
		year = parseYear(yrStr)
	}

	// Рядки таблиці «<ul class="full_info"><li><span>Жанр:</span> <a…>».
	//
	// Реальна розмітка eneyida.tv — це голі <li> усередині ul.full_info,
	// а не .full_info-item. Значення лежить у тих самих <a> (або просто
	// текстом для «Тривалість:»), тому окремо беремо текст рядка без
	// мітки й окремо — посилання.
	doc.Find(".full_info-item, .fi-row, ul.full_info li, ul.full-info li").Each(func(i int, s *goquery.Selection) {
		labelSel := s.Find(".fi-label, span:first-child").First()
		labelText := strings.TrimSpace(labelSel.Text())
		if labelText == "" {
			return
		}
		label := strings.ToLower(labelText)

		// Значення = весь текст рядка мінус мітка з початку. Для
		// «<li><span>Тривалість:</span> 00:50:00</li>» це «00:50:00».
		valText := strings.TrimSpace(s.Text())
		valText = strings.TrimSpace(strings.TrimPrefix(valText, labelText))

		// Окремий .fi-value є лише в старішій розмітці .full_info-item.
		// У новій його немає, а span:last-child впав би на саму мітку,
		// тому перекриваємо лише справжнім .fi-value.
		if v := strings.TrimSpace(s.Find(".fi-value").First().Text()); v != "" {
			valText = v
		}

		switch {
		case strings.Contains(label, "режис") && director == "":
			director = joinDetailList(s, valText)
		// На eneyida.tv мітка рядка акторів — «В ролях:», а не «Актори:».
		case (strings.Contains(label, "актор") || strings.Contains(label, "ролях")) && len(actors) == 0:
			actors = append(actors, detailAnchors(s, valText)...)
		case strings.Contains(label, "жанр") && len(genres) == 0:
			genres = append(genres, detailAnchors(s, valText)...)
		case strings.Contains(label, "країн") && len(countries) == 0:
			countries = append(countries, detailAnchors(s, valText)...)
		case strings.Contains(label, "рік") && year == 0:
			year = parseYear(valText)
		case strings.Contains(label, "трива") && duration == "":
			duration = strings.TrimSpace(valText)
		}
	})

	if year == 0 {
		year = parseYear(doc.Find(".full_info, .full-info").Text())
	}

	var rating float64
	ratingText := doc.Find(".r_imdb, .rating, .imdb-rate, .full-rates").Text()
	m := reRatingNum.FindString(ratingText)
	if m != "" {
		if r, err := strconv.ParseFloat(m, 64); err == nil {
			rating = r
		}
	}

	// Тип матеріалу. На сторінці опису він лежить у рядку «Жанр:» —
	// перше посилання веде на сам розділ (/series/, /anime/, /cartoon/,
	// /cartoon-series/). URL сторінки кореневий, тож фолбек за ним
	// безкоштовний, але ніколи не спрацьовує.
	mediaType := eneyidaTypeFromHref(itemURL)
	doc.Find("ul.full_info li, ul.full-info li, .full_info-item, .fi-row").Each(func(i int, s *goquery.Selection) {
		if !strings.Contains(strings.ToLower(strings.TrimSpace(s.Find(".fi-label, span:first-child").Text())), "жанр") {
			return
		}
		href, ok := s.Find("a").First().Attr("href")
		if !ok {
			return
		}
		mediaType = eneyidaTypeFromHref(href)
	})

	details := &domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:            itemURL,
			ProviderID:    p.ID(),
			Title:         title,
			OriginalTitle: origTitle,
			PosterURL:     poster,
			Year:          year,
			Rating:        rating,
			Type:          mediaType,
			URL:           itemURL,
		},
		Description: desc,
		Genres:      genres,
		Countries:   countries,
		Director:    director,
		Actors:      actors,
		Duration:    duration,
	}

	// Сезони й озвучки з дерева PlayerJS-плейлиста.
	//
	// На eneyida.tv плейлист живе НЕ на сторінці опису, а в iframe
	// hdvbua.pro/embed/… (перевірено живим запитом: саме там
	// file:'[{"title":"1 сезон","folder":[{"title":"Цікава Ідея",…'),
	// тому доводиться завантажити кандидата плеєра. Якщо розібрати не
	// вдалося — details лишається без сезонів, краще ніж вигадані.
	applyPlaylistDetails(ctx, p.client, p.baseURL, details, html, itemURL)

	return details, nil
}

// joinDetailList склеює значення рядка: якщо є посилання — беруться
// їхні тексти, інакше — сирий текст без мітки.
func joinDetailList(row *goquery.Selection, fallback string) string {
	if items := detailAnchors(row, ""); len(items) > 0 {
		return strings.Join(items, ", ")
	}
	return strings.TrimSpace(fallback)
}

// detailAnchors повертає тексти посилань рядка, а якщо їх немає —
// окремий fallback-текст. Значення на eneyida розділені «•» і «,»,
// тому сирий текст доводиться розбивати.
func detailAnchors(row *goquery.Selection, fallback string) []string {
	var out []string
	row.Find("a").Each(func(_ int, a *goquery.Selection) {
		if t := strings.Trim(strings.TrimSpace(a.Text()), " •·|"); t != "" {
			out = append(out, t)
		}
	})
	if len(out) > 0 {
		return out
	}
	fallback = strings.Trim(strings.TrimSpace(fallback), " •·|")
	if fallback == "" {
		return nil
	}
	for _, part := range strings.FieldsFunc(fallback, func(r rune) bool {
		return r == ',' || r == '•' || r == '·' || r == '|' || r == ';'
	}) {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// mergePlaylistSeasons зводить сезони, зібрані BuildSeasonsFromPlaylist,
// до одного списку за номерами.
//
// BuildSeasonsFromPlaylist групує листя за (озвучка, сезон), бо саме
// так episode-и розрізняються між студіями. Але в MediaDetails.Seasons
// це дає дублікати: для серіалу з двома озвучками клієнт бачив би
// «Сезон 1» двічі, а «Сезон 2» двічі. Об'єднуємо за номером, а
// повний (необ'єднаний) набір сезонів лишається всередині кожного
// Voiceover — саме він потрібен, щоб GetStreams знайшов правильну озвучку.
func mergePlaylistSeasons(seasons []domain.Season) []domain.Season {
	if len(seasons) == 0 {
		return nil
	}

	order := make([]int, 0, len(seasons))
	byNumber := map[int]*domain.Season{}
	seenEpisode := map[int]map[int]bool{}

	for _, s := range seasons {
		cur, ok := byNumber[s.Number]
		if !ok {
			cp := domain.Season{Number: s.Number, Title: s.Title}
			byNumber[s.Number] = &cp
			seenEpisode[s.Number] = map[int]bool{}
			order = append(order, s.Number)
			cur = &cp
		}
		for _, ep := range s.Episodes {
			if seenEpisode[s.Number][ep.Number] {
				continue
			}
			seenEpisode[s.Number][ep.Number] = true
			cur.Episodes = append(cur.Episodes, ep)
		}
	}

	out := make([]domain.Season, 0, len(order))
	for _, n := range order {
		eps := byNumber[n].Episodes
		sort.SliceStable(eps, func(i, j int) bool { return eps[i].Number < eps[j].Number })
		out = append(out, *byNumber[n])
	}
	return out
}

// applyPlaylistDetails заповнює Seasons/Voiceovers деталей, якщо вдалося
// розібрати дерево PlayerJS-плейлиста.
//
// Спільна інфраструктура для DLE-провайдерів (uakino/eneyida/lavakino):
// дерево може лежати прямо в HTML сторінки опису, а може — в iframe
// плеєра (eneyida). Тому спершу пробуємо локально, а за відсутності
// file: завантажуємо найкращих кандидатів плеєра (обмежено
// dlePlaylistProbeLimit і тим самим playerCandidateTimeout, що й у
// GetStreams).
func applyPlaylistDetails(ctx context.Context, client *TLSClient, siteBaseURL string, details *domain.MediaDetails, html, itemURL string) {
	if details == nil {
		return
	}
	items, ok := playlistFromItemPage(ctx, client, siteBaseURL, html, itemURL)
	if !ok {
		return
	}
	details.Seasons = mergePlaylistSeasons(BuildSeasonsFromPlaylist(items))
	details.Voiceovers = BuildVoiceoversFromPlaylist(items)
}

// dlePlaylistProbeLimit — скільки iframe-кандидатів плеєра завантажує
// GetDetails у пошуках плейлиста. Більше не потрібно: перший-ліпший
// плеєр уже містить або сезони, або озвучки.
const dlePlaylistProbeLimit = 3

// playlistFromItemPage дістає дерево PlayerJS-плейлиста зі сторінки
// опису або з найкращих iframe-кандидатів на ній.
func playlistFromItemPage(ctx context.Context, client *TLSClient, siteBaseURL, html, itemURL string) ([]playerJSPlaylistItem, bool) {
	if items, ok := ParsePlayerJSPlaylist(html); ok {
		return items, true
	}

	candidates := rankPlayerCandidates(html, itemURL)
	if len(candidates) > dlePlaylistProbeLimit {
		candidates = candidates[:dlePlaylistProbeLimit]
	}

	for _, cand := range candidates {
		candCtx, cancel := context.WithTimeout(ctx, playerCandidateTimeout)
		page, err := client.GetNoCache(candCtx, cand.URL, siteBaseURL)
		cancel()
		if err != nil {
			continue
		}
		if items, ok := ParsePlayerJSPlaylist(page); ok {
			return items, true
		}
	}
	return nil, false
}

// GetStreams знаходить плеєр у iframe та резолвить його у прямий медіа-потік.
//
// Раніше метод повертав URL самого iframe — це HTML, який libmpv не демодулює
// ("Failed to recognize file format."). Тепер iframe розбирається через
// ResolvePlayerHTML, і за відсутності розпізнаного потоку повертається
// порожній список + ErrUnresolvablePlayer (а не HTML-сторінка для mpv).
// ONE budget covers the item-page fetch AND the iframe fan-out — see the
// identical note in uakino.GetStreams for the trade-off rationale.
func (p *EneyidaProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, PlayerResolveTimeout)
	defer cancel()

	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida get streams: %w", err)
	}

	return resolveStreamsFromItemPage(ctx, p.client, p.ID(), itemURL, html, season, episode, voiceID)
}
