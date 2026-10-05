package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLavakinoProviderBasic(t *testing.T) {
	p := NewLavakinoProvider(nil)
	if p.ID() != "lavakino" {
		t.Errorf("expected ID 'lavakino', got '%s'", p.ID())
	}
	if p.Name() != "Lavakino" {
		t.Errorf("expected Name 'Lavakino', got '%s'", p.Name())
	}
	if p.BaseURL() != "https://lavakino.net" {
		t.Errorf("expected BaseURL 'https://lavakino.net', got '%s'", p.BaseURL())
	}
	desc := p.Describe()
	if !desc.ShowOnHome {
		t.Errorf("expected ShowOnHome true")
	}
	if !desc.SearchEnabledDefault {
		t.Errorf("expected SearchEnabledDefault true")
	}
}

// Живі перевірки lavakino.net (Chrome120UserAgent + Referer, 2026-10):
//
//	/filmys/       200
//	/serialy/      200
//	/cartoonss/    200
//	/anime/        200
//	/films/        404   <- НЕ існує
//	/series/       404   <- НЕ існує
func TestLavakinoGetSection(t *testing.T) {
	p := NewLavakinoProvider(nil)
	tests := []struct {
		contentType string
		wantSection string
	}{
		{"movie", "filmys"},
		{"series", "serialy"},
		{"cartoon", "cartoonss"},
		{"anime", "anime"},
		{"other", ""},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := p.getSection(tt.contentType)
			if got != tt.wantSection {
				t.Errorf("getSection(%q) = %q, want %q", tt.contentType, got, tt.wantSection)
			}
		})
	}
}

func TestLavakinoGetSectionNeverReturnsDeadSlugs(t *testing.T) {
	p := NewLavakinoProvider(nil)
	dead := map[string]bool{"films": true, "series": true, "cartoon": true}
	for _, ct := range []string{"movie", "series", "cartoon", "anime"} {
		if got := p.getSection(ct); dead[got] {
			t.Errorf("getSection(%q) = %q — 404 на живому lavakino.net", ct, got)
		}
	}
}

// Реальні слади lavakino.net відрізняються від eneyida/uakino, тому
// класифікація окрема.
func TestLavakinoTypeByPath(t *testing.T) {
	cases := []struct {
		href string
		want string
	}{
		{"https://lavakino.net/filmys/34033-misiia.html", "movie"},
		{"https://lavakino.net/serialy/123-x.html", "series"},
		{"https://lavakino.net/cartoonss/456-x.html", "cartoon"},
		{"https://lavakino.net/anime/789-x.html", "anime"},
		// Старі шляхи, які теж треба впізнавати.
		{"https://lavakino.net/mult/2-x.html", "cartoon"},
		{"https://lavakino.net/serial/1-x.html", "series"},
		{"https://lavakino.net/cartoon/3-x.html", "cartoon"},
		// Мертві слади eneyida/uakino тут не мають нічого.
		{"https://lavakino.net/series/1-x.html", "movie"},
		{"https://lavakino.net/films/1-x.html", "movie"},
		{"https://lavakino.net/123-x.html", "movie"},
	}
	for _, tc := range cases {
		t.Run(tc.href, func(t *testing.T) {
			if got := lavakinoTypeByPath(tc.href); got != tc.want {
				t.Errorf("lavakinoTypeByPath(%q) = %q, want %q", tc.href, got, tc.want)
			}
		})
	}
}

// Сезони й озвучки з дерева PlayerJS-плейлиста.
func TestLavakinoGetDetailsFillsSeasonsAndVoiceovers(t *testing.T) {
	page := `<!DOCTYPE html><html><body><h1>Серіал</h1><script>
var player = new Playerjs({file: '[{"title":"Сезон 1","folder":[{"title":"1+1","folder":[{"title":"1 серія","file":"https://cdn.example/hls/ser.s01e01/hls/index.m3u8"}]},{"title":"MoonAnime","folder":[{"title":"1 серія","file":"https://cdn.example/hls/ser.s01e01.moon/hls/index.m3u8"}]}]}]'});
</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	p := &LavakinoProvider{client: covTLS(t), baseURL: srv.URL}
	d, err := p.GetDetails(context.Background(), srv.URL+"/serialy/123-x.html")
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}
	if len(d.Seasons) != 1 || len(d.Seasons[0].Episodes) != 1 {
		t.Fatalf("expected 1 season with 1 merged episode, got %+v", d.Seasons)
	}
	if d.Seasons[0].Number != 1 {
		t.Errorf("season number = %d, want 1", d.Seasons[0].Number)
	}
	if len(d.Voiceovers) != 2 {
		t.Fatalf("expected 2 voiceovers, got %+v", d.Voiceovers)
	}
	names := map[string]bool{}
	for _, v := range d.Voiceovers {
		names[v.Name] = true
		if v.ID == "" {
			t.Errorf("voiceover without id: %+v", v)
		}
	}
	if !names["1+1"] || !names["MoonAnime"] {
		t.Errorf("voiceovers = %v, want 1+1 and MoonAnime", names)
	}
}

func TestLavakino404ReturnsEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	tls := covTLS(t)
	p := &LavakinoProvider{
		client:  tls,
		baseURL: srv.URL,
	}

	items, err := p.GetPopular(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items on 404, got %d", len(items))
	}

	catItems, err := p.GetByCategory(context.Background(), "drama", "movie", 1)
	if err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
	if len(catItems) != 0 {
		t.Fatalf("expected 0 items on 404, got %d", len(catItems))
	}
}

