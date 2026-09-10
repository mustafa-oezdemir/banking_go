-- Ledger entries are the authoritative financial history. Corrections must be
-- represented by compensating entries; mutating or deleting existing rows
-- would silently invalidate balances, reconciliation, and audit evidence.
CREATE OR REPLACE FUNCTION reject_ledger_entry_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'ledger entries are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_entries_append_only
BEFORE UPDATE OR DELETE ON entries
FOR EACH ROW EXECUTE FUNCTION reject_ledger_entry_mutation();
