-- 000005_remove_hdrezka.up.sql
-- Видалення рядків провайдера HDRezka: провайдер повністю прибрано з реєстру на стороні сервера,
-- тому його дані більше не можуть бути синхронізовані з клієнтами (sync_handler відтворює
-- локальні рядки з provider_id='hdrezka'). Міграція forward-only та ідемпотентна.
DELETE FROM favorites      WHERE provider_id = 'hdrezka';
DELETE FROM watch_history  WHERE provider_id = 'hdrezka';
DELETE FROM content_cache  WHERE provider_id = 'hdrezka';