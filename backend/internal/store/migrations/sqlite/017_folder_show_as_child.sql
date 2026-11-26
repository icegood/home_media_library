-- A folder can be hidden from its parent: it is then left out of the parent's
-- folder list, timelines, maps and stats, and only admins can open it (from
-- the admin page). Every existing folder stays visible.
ALTER TABLE media_folders ADD COLUMN show_as_child NUMERIC NOT NULL DEFAULT 1;
