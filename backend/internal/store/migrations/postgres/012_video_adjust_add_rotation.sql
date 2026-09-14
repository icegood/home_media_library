DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_name = 'video_adjust' AND column_name = 'rotation') THEN
    ALTER TABLE video_adjust ADD COLUMN rotation INTEGER NOT NULL DEFAULT 0;
  END IF;
END $$;