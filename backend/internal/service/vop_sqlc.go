package service

import (
	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// Kept in a tiny adapter so the VoP algorithm remains easy to read and test.
func sqlcGetBeneficiaryParams(ownerID uuid.UUID, iban string) sqlc.GetBeneficiaryByIBANParams {
	return sqlc.GetBeneficiaryByIBANParams{OwnerID: ownerID, Iban: iban}
}
