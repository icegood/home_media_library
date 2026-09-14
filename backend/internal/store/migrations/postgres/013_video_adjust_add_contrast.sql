DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_name = 'video_adjust' AND column_name = 'contrast') THEN
    ALTER TABLE video_adjust ADD COLUMN contrast DOUBLE PRECISION NOT NULL DEFAULT 1.0;
  END IF;
END $$;