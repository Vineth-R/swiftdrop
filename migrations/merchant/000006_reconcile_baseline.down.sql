-- Reconciliation retains data from two historical chains and is forward-only.
DO $$ BEGIN
  RAISE EXCEPTION 'merchant baseline reconciliation is forward-only; use a reviewed forward migration';
END $$;
