-- Trigger function for variant_combinations
--
-- Upserts into variant_combinations and sets the FK on prow_jobs.
-- Only NULL variants produce a NULL variant_combination_id; empty
-- arrays ('{}') get their own entry.
--
-- The variant_combinations table, prow_jobs.variant_combination_id
-- column, and FK constraint are created by GORM AutoMigrate. The
-- trigger is attached by ensureVariantCombinationTrigger after
-- AutoMigrate ensures prow_jobs exists.

CREATE OR REPLACE FUNCTION set_variant_combination_id()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.variants IS NOT NULL THEN
        INSERT INTO variant_combinations (variants)
        VALUES (NEW.variants)
        ON CONFLICT (variants) DO UPDATE SET variants = EXCLUDED.variants
        RETURNING id
        INTO NEW.variant_combination_id;
    ELSE
        NEW.variant_combination_id := NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
