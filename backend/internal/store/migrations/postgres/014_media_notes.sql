DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_name = 'media' AND column_name = 'notes') THEN
    ALTER TABLE media ADD COLUMN notes TEXT NOT NULL DEFAULT '';
  END IF;
END $$;