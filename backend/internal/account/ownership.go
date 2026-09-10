package account

import "github.com/google/uuid"

// CustomerCanOperate reports whether a non-system account is owned by the
// authenticated customer. Identifier unpredictability is never authorization.
func CustomerCanOperate(requesterID, ownerID uuid.UUID, ownerAssigned, systemAccount bool) bool {
	return requesterID != uuid.Nil && ownerAssigned && !systemAccount && ownerID == requesterID
}
