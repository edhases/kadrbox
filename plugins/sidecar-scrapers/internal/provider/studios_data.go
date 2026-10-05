package provider

// allStudios повертає словник українських студій озвучення,
// телеканалів та фандаб-команд.
//
// Дані — публічні назви студій дубляжу, зібрані з назв у плеєрах
// ashdi.vip, hdvbua.pro, zetvideo.net та з каталогів mikai. Усі
// роздільники збережені з оригіналу, бо вони відповідають на
// реальне питання «це телеканал, це кінодубляж, чи це фандаб».
//
// Зауваження щодо покриття: словник далекий від вичерпного.
// Реальні плеєри приносять назви, яких тут немає, — «Незупиняй»
// від групи 1+1, «Студія Гдерек», «Краків». Тому будь-яка невідома
// назва не губиться: ResolveStudio повертає ok=false, і клієнт
// показує сиру назву як є. Це навмисно — краще «Незупиняй» у
// списку, ніж порожній рядок або вигаданий ідентифікатор.
func allStudios() []studioEntry {
	return []studioEntry{
		// --- Телеканали та офіційні мовники ---
		{id: "1plus1", name: "1+1", aliases: []string{"1 плюс 1", "один плюс один"}},
		{id: "2plus2", name: "2+2", aliases: []string{"2 плюс 2", "два плюс два"}},
		{id: "tet", name: "ТЕТ"},
		{id: "plusplus", name: "Плюс-Плюс", aliases: []string{"плюсплюс", "plus plus"}},
		{id: "ictv", name: "ICTV", aliases: []string{"айсітіві"}},
		{id: "ictv2", name: "ICTV2", aliases: []string{"ictv 2"}},
		{id: "novyi", name: "Новий канал", aliases: []string{"новий"}},
		{id: "stb", name: "СТБ"},
		{id: "qtv", name: "QTV", aliases: []string{"куй тб"}},
		{id: "inter", name: "Інтер"},
		{id: "k1", name: "К1", aliases: []string{"к1"}},
		{id: "ntn", name: "НТН"},
		{id: "ukraina", name: "ТРК Україна", aliases: []string{"україна", "канал україна"}},
		{id: "nlotv", name: "НЛО-ТВ", aliases: []string{"нло-тв", "нло.tv", "нло тб"}},
		{id: "suspilne", name: "Суспільне", aliases: []string{"суспільне культура", "перший", "ua:перший"}},
		{id: "dim", name: "Дім", aliases: []string{"дім (телеканал)"}},
		{id: "cineplus", name: "Cine+", aliases: []string{"сіне плюс", "cine plus", "cine-plus"}},
		{id: "paramountcomedy", name: "Paramount Comedy", aliases: []string{"comedy central", "парамаунт"}},
		{id: "amc", name: "AMC", aliases: []string{"амс"}},
		{id: "megogo", name: "Megogo", aliases: []string{"мегого", "megogo voice"}},
		{id: "sweettv", name: "Sweet.tv", aliases: []string{"світ тв"}},
		{id: "netflix", name: "Netflix", aliases: []string{"нетфлікс"}},
		{id: "pershyi", name: "Перший", aliases: []string{"ua:перший"}},

		// --- Офіційні студії кінодубляжу ---
		{id: "postmodern", name: "Postmodern", aliases: []string{"постмодерн", "postmodern postproduction"}},
		{id: "ledoyen", name: "LeDoyen", aliases: []string{"ледоєн", "ледоен", "ле доєн", "le doyen", "le-doyen"}},
		{id: "taktreba", name: "Так Треба Продакшн", aliases: []string{"так треба", "tak treba", "так треба продакшн", "тактребапродакшн", "ttp"}},
		{id: "cinemasound", name: "Cinema Sound Production", aliases: []string{"cinema sound", "сінема саунд", "синема саунд"}},

		// --- Студії озвучки фільмів і серіалів ---
		{id: "dniprofilm", name: "DniproFilm", aliases: []string{"дніпрофільм", "дніпрофілм"}},
		{id: "tsikavaideya", name: "Цікава Ідея", aliases: []string{"цікава ідея"}},
		{id: "v_odneprylo", name: "В одне рило", aliases: []string{"в_одне_рило"}},
		{id: "uaflix", name: "UaFlix", aliases: []string{"уафлікс"}},
		{id: "hdrezka", name: "HDrezka Studio", aliases: []string{"резка"}},
		{id: "bamboo", name: "BambooUA", aliases: []string{"bambooua", "бамбу"}},
		{id: "ukrdub", name: "UkrDub", aliases: []string{"укрдаб"}},
		{id: "blueberry", name: "Blueberry Studio", aliases: []string{"блюберрі студіо"}},
		{id: "sunnysiders", name: "Sunnysiders", aliases: []string{"санісайдерс"}},
		{id: "baibako", name: "BaibaKo", aliases: []string{"baibakotv", "байбако", "бабайко"}},
		{id: "ozz", name: "OZZ", aliases: []string{"озз"}},
		{id: "omikron", name: "Омікрон"},
		{id: "robotagolosom", name: "Робота Голосом", aliases: []string{"роботаголосом", "рідний голос"}},
		{id: "unimay", name: "Unimay", aliases: []string{"унімей", "днімей"}},
		{id: "kachur", name: "Студія Качур", aliases: []string{"качур", "kachur studio"}},
		{id: "hatoshi", name: "HATOSHI", aliases: []string{"хатоші"}},
		{id: "maysternyasliv", name: "Майстерня Слів", aliases: []string{"майстерняслів", "майстерня слiв"}},
		{id: "kit", name: "КІТ", aliases: []string{"студія кіт"}},
		{id: "yaniam", name: "Yaniam", aliases: []string{"яніам"}},
		{id: "nezupinyai", name: "Незупиняй", aliases: []string{"не зупиняй", "nezupinyay"}},

		// --- Аніме-студії та фандаб-команди ---
		{id: "fanvoxua", name: "FanVoxUA", aliases: []string{"фанвоксюа", "фанвокс"}},
		{id: "amanogawa", name: "Amanogawa", aliases: []string{"аманогава"}},
		{id: "didko", name: "Didko Studio", aliases: []string{"дідько"}},
		{id: "ufdub", name: "UFDUB", aliases: []string{"юфдаб"}},
		{id: "kaizoku", name: "Клан Кайзоку", aliases: []string{"clan kaizoku", "кайзоку"}},
		{id: "glassmoon", name: "Glass Moon", aliases: []string{"глас мун"}},
		{id: "aniua", name: "AniUA", aliases: []string{"аніюа"}},
		{id: "animesh", name: "Animesh", aliases: []string{"анімеш"}},
		{id: "kioto", name: "Кіото", aliases: []string{"kioto anime"}},
		{id: "dzuski", name: "Dzuski", aliases: []string{"дзуські"}},
		{id: "cloverdub", name: "CloverDUB", aliases: []string{"кловердаб"}},
		{id: "inari", name: "Inari", aliases: []string{"inaridub"}},
		{id: "inariokami", name: "InariOkami", aliases: []string{"інарі окамі"}},
		{id: "melvoice", name: "MelodicVoiceStudio", aliases: []string{"melvoice"}},
		{id: "animeclassic", name: "Anime Classic", aliases: []string{"аніме класік"}},
		{id: "moonanime", name: "MoonAnime", aliases: []string{"мунаніме"}},
		{id: "legat", name: "Legat", aliases: []string{"легат"}},
		{id: "mikai", name: "Mikai", aliases: []string{"мікай"}},
		{id: "tatakae", name: "TATAKAE"},
		{id: "dalibude", name: "Далі буде"},
		{id: "and5", name: "AND5 Studio"},
		{id: "anifanua", name: "AniFanUA", aliases: []string{"аніфанюа"}},
		{id: "anikoe", name: "AniKoe", aliases: []string{"анікое"}},
		{id: "animeoriginal", name: "AnimeOriginal"},
		{id: "beysub", name: "Beysub Studio", aliases: []string{"бейсаб"}},
		{id: "borshdub", name: "BorshDUB", aliases: []string{"борщдаб"}},
		{id: "crystal_shade", name: "Crystal Shade", aliases: []string{"крістал шейд"}},
		{id: "espada", name: "Espada Studio", aliases: []string{"еспада"}},
		{id: "flayzer", name: "Flayzer", aliases: []string{"флейзер"}},
		{id: "futashine", name: "Futashine"},
		{id: "hajimedub", name: "HajimeDUB", aliases: []string{"хаджімедаб"}},
		{id: "k0wbassa", name: "k0wbassa", aliases: []string{"ковбаса"}},
		{id: "kafori", name: "Kafori", aliases: []string{"кафорі"}},
		{id: "kawaii_dub", name: "Kawaii Dub", aliases: []string{"каваі даб"}},
		{id: "kitsune", name: "Kitsune", aliases: []string{"кіцуне"}},
		{id: "lifecycle", name: "Life Cycle", aliases: []string{"лайф сайкл"}},
		{id: "lvp", name: "LVP"},
		{id: "milki_dub", name: "Milki-Dub", aliases: []string{"мілкі даб"}},
		{id: "modeo", name: "Modeo"},
		{id: "mogi", name: "Mogi", aliases: []string{"могі"}},
		{id: "morys", name: "Morys", aliases: []string{"моріс"}},
		{id: "mrcrashfox", name: "MrCrashFox", aliases: []string{"крашфокс"}},
		{id: "p1rsti", name: "p1rsti", aliases: []string{"персти"}},
		{id: "raccoonhouse", name: "RaccoonHouse", aliases: []string{"ракунхаус"}},
		{id: "ryukastudio", name: "Ryuka Studio", aliases: []string{"рюка"}},
		{id: "salovpalo", name: "Сало Впало"},
		{id: "shield_team", name: "Shield Team", aliases: []string{"шілд тім"}},
		{id: "shiwa", name: "Shiwa", aliases: []string{"шива"}},
		{id: "shogun", name: "Shogun", aliases: []string{"сьогун"}},
		{id: "togarashi", name: "Togarashi", aliases: []string{"тогараші"}},
		{id: "sviydub", name: "СвійDUB", aliases: []string{"свій даб", "svij dub", "svijdub"}},
		{id: "10gu", name: "10GU", aliases: []string{"10 гу"}},
		{id: "4ua", name: "4UA", aliases: []string{"4 юа"}},
		{id: "aleksalo", name: "AleksAlo", aliases: []string{"алексало"}},
		{id: "boku_no_pidval", name: "Боку но підвал", aliases: []string{"boku no pidval"}},

		// --- Субтитри ---
		// Запис «субтитри» навмисно останній: resolveStudioInfo в
		// оригіналі пропускає його в циклі, щоб загальне слово не
		// перехопило «СвійSUB» та «UaAniSub». У нас той самий
		// ефект досягається тим, що точний збіг перевіряється
		// першим, а підрядковий шукає найдовший аліас.
		{id: "uaanisub", name: "UaAniSub", aliases: []string{"ua ani sub", "уаанісаб", "юаанісаб"}},
		{id: "svijsub", name: "СвійSUB", aliases: []string{"svijsub", "svij sub", "свійсаб"}},
		{id: "chornyi_veres", name: "Чорний Верес"},
		{id: "project_ua_lines", name: "Project U&A Lines", aliases: []string{"u&a lines"}},
		{id: "anitube", name: "AniTube", aliases: []string{"анітюб"}},
		{id: "subtitles", name: "Субтитри"},
	}
}