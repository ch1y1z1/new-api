package websearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_Tavily(t *testing.T) {
	p := NewProvider("tavily", "test-key", "")
	require.NotNil(t, p)
	assert.Equal(t, "tavily", p.Name())
}

func TestNewProvider_Brave(t *testing.T) {
	p := NewProvider("brave", "test-key", "")
	require.NotNil(t, p)
	assert.Equal(t, "brave", p.Name())
}

func TestNewProvider_SearXNG(t *testing.T) {
	p := NewProvider("searxng", "", "http://localhost:8080")
	require.NotNil(t, p)
	assert.Equal(t, "searxng", p.Name())
}

func TestNewProvider_Unknown(t *testing.T) {
	p := NewProvider("unknown", "", "")
	assert.Nil(t, p)
}

func TestNewProvider_Empty(t *testing.T) {
	p := NewProvider("", "", "")
	assert.Nil(t, p)
}
