package account

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCustomerCanOperateEnforcesOwnershipBoundary(t *testing.T) {
	ownerID := uuid.New()
	assert.True(t, CustomerCanOperate(ownerID, ownerID, true, false))
	assert.False(t, CustomerCanOperate(uuid.New(), ownerID, true, false))
	assert.False(t, CustomerCanOperate(ownerID, ownerID, false, false))
	assert.False(t, CustomerCanOperate(ownerID, ownerID, true, true))
	assert.False(t, CustomerCanOperate(uuid.Nil, ownerID, true, false))
}
