-- =====================================================================
-- SwiftDrop local database bootstrap
-- Runs once when the PostgreSQL data directory is empty.
-- =====================================================================

CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS pricing;
CREATE SCHEMA IF NOT EXISTS parcel;
CREATE SCHEMA IF NOT EXISTS merchant;
CREATE SCHEMA IF NOT EXISTS notification;

-- Shared helper used by the baseline table migrations to maintain updated_at.
CREATE OR REPLACE FUNCTION public.set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
