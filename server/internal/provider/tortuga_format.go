package provider

import (
	"regexp"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// Розбір формату file: від Tortuga.
//
// Спостерігається на живому трафіку (tortuga.tw/embed/{id}),
// знято 2026-10-05 на прикладі «Величне століття. Роксолана»:
//
//	{1+1}https://calypso.tortuga.tw/hls/serials/muhtesem.yuzyil/s01/
//	     muhtesem.yuzyil.s01e01.1plus1.mvo_66961/hls/index.m3u8(subtitle:)
//
// Тобто назва озвучки стоїть у фігурних дужках перед URL, а
// субтитри — у круглих після нього. Це єдиний випадок у нашій
// практиці, коли назва студії приходить не з title вузла дерева,
// а з самого значення файлу. Через це загальний обхід дерева не
// бачив озвучки для Tortuga — і клієнт отримував порожній
// селектор.
//
// Окремо важливо: без цієї розшифровки автор uafilms/core вирішив,
// що «Tortuga офлайн», і викреслив джерело зі свого коду. CDN
// живий, просто його формат не збігався з їхнім парсером.

var (
	// reTortugaDub — назва озвучки між { і } на початку значення.
	reTortugaDub = regexp.MustCompile(`^\s*\{([^{}]*)\}`)
	// reTortugaSuffix — хвіст (subtitle:...) після URL.
	reTortugaSuffix = regexp.MustCompile(`\(([^()]*)\)\s*$`)
)

// ParseTortugaFileField розбирає значення file: від Tortuga на
// озвучку, URL та субтитри.
//
// dub порожній, якщо фігурних дужок немає — таке трапляється на
// кінематографічних трейлерах і на старих сторінках, де формат не
// встигли перейти. Функція тоді просто повертає весь рядок як URL,
// щоб не втратити єдине доступне джерело.
func ParseTortugaFileField(raw string) (dub, mediaURL, subtitle string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", ""
	}

	// 1. Назва озвучки з префікса.
	if m := reTortugaDub.FindStringSubmatch(s); m != nil {
		dub = cleanStudioName(m[1])
		s = strings.TrimSpace(s[len(m[0]):])
	}

	// 2. Хвіст із субтитрами. Знімаємо ДОURL-парсингу, бо дужки
	//    ніколи не є частиною URL.
	if m := reTortugaSuffix.FindStringSubmatch(s); m != nil {
		body := strings.TrimSpace(m[1])
		// Очікуємо «subtitle:<url>» або «sub:<url>». Якщо
		// префікса немає, тоді це, ймовірно, частина URL — не
		// чіпаємо.
		if i := strings.Index(body, ":"); i > 0 {
			switch strings.ToLower(strings.TrimSpace(body[:i])) {
			case "subtitle", "sub", "субтитри", "субтитр":
				subtitle = strings.TrimSpace(body[i+1:])
			}
		}
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	}

	return dub, s, subtitle
}

// ApplyTortugaDub дописує озвучку з файлу в контекст листа, якщо
// дерево її не дало.
//
// Допомогає для провайдерів, які віддають плеєр Tortuga: вони
// отримують озвучку з розшифрованого file:, тоді як контекст
// обходу лишається порожнім. Пріоритет на боці контексту: якщо
// title вузла вже дав студію, вона авторитетніша за значення
// файлу.
func ApplyTortugaDub(ctx *PlaylistContext, rawFile string) {
	if ctx == nil || ctx.Dub != "" {
		return
	}
	if dub, _, _ := ParseTortugaFileField(rawFile); dub != "" {
		ctx.Dub = dub
	}
}

// ApplyTortugaSubtitle повертає субтитри з файлу, якщо дерево їх не
// дало окремим полем.
func ApplyTortugaSubtitle(rawFile string) []domain.SubtitleSource {
	_, _, sub := ParseTortugaFileField(rawFile)
	if sub == "" {
		return nil
	}
	return parseSubtitlesFromPlayerHTML(sub)
}