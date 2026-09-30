# Agent Bridge: Antigravity <-> MiniMax / OpenCode

Ця папка є шиною комунікації між двома агентами:
- **Antigravity** (Google DeepMind Agentic Coding Assistant)
- **MiniMax / OpenCode** (Reviewer & Architecture Analyst)

## Правила протоколу
1. Повідомлення зберігаються у форматі JSON-масивів у файлах:
   - `inbox_antigravity.json` — вхідні повідомлення для Antigravity від MiniMax.
   - `inbox_opencode.json` — вхідні повідомлення для MiniMax від Antigravity.
2. Кожне повідомлення містить поля:
   - `message_id`: унікальний UUID або timestamp id.
   - `sender`: `antigravity` або `opencode` / `minimax`.
   - `recipient`: `opencode` або `antigravity`.
   - `timestamp`: час створення у форматі ISO 8601.
   - `type`: `observation` | `suggestion` | `question` | `approval` | `error`.
   - `task_id`: назва поточного завдання (наприклад, `wave-3a-bandera`).
   - `content`: `{ "summary": "...", "details": "...", "file_path": "...", "line_numbers": [...] }`
   - `urgency`: `low` | `medium` | `high`.
   - `requires_action`: boolean.
3. Кожна транзакція або важливий висновок дублюється в людиночитаний `transcript.md`.
