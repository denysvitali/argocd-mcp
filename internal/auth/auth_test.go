package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskToken_ShortToken(t *testing.T) {
	result := MaskToken("123")
	assert.Equal(t, "****", result)
}

func TestMaskToken_NormalToken(t *testing.T) {
	result := MaskToken("longtoken12345")
	assert.Equal(t, "long****2345", result)
}

func TestMaskToken_EmptyToken(t *testing.T) {
	result := MaskToken("")
	assert.Equal(t, "****", result)
}
