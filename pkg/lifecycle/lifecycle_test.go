package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	m := NewManager(nil, nil, nil, nil, nil)
	assert.NotNil(t, m)
	assert.Nil(t, m.ITerm())
	assert.Nil(t, m.State())
	assert.Nil(t, m.Trust())
}

func TestNewManagerWithLogger(t *testing.T) {
	m := NewManager(nil, nil, nil, nil, nil)
	// Should not panic - nopLogger is used
	assert.NotNil(t, m.logger)
}
