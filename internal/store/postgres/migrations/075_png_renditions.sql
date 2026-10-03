-- PNG uses the existing immutable rendition envelope and retention authority.
-- The application validates canonical base64 and bounded decoded image bytes.
ALTER TABLE chartworks.render_renditions DROP CONSTRAINT render_renditions_format_check;
ALTER TABLE chartworks.render_renditions ADD CONSTRAINT render_renditions_format_check
 CHECK(format IN ('json','csv','html','svg','png'));
