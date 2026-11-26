package main

import (
	"errors"
	"testing"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
)

// A ui_host edit must reach traces_url even while collects keep failing.
func TestDegradeAppliesCurrentIdentityOverCache(t *testing.T) {
	cache := newLastGood()
	cache.record("demo", pluginapi.PropertyMap{
		propertyKeyProjectID: "cm0demo",
		propertyKeyHost:      "https://langfuse.example.com",
		propertyKeyScope:     "",
		propertyKeyTracesURL: "https://langfuse.example.com/project/cm0demo/traces",
		propertyKeyTotalCost: "4.5",
	})

	project := testProject("https://langfuse.example.com")
	project.UIHost = "https://ui.example.com"

	assert.Equal(t, pluginapi.PropertyMap{
		propertyKeyProjectID:  "cm0demo",
		propertyKeyHost:       "https://langfuse.example.com",
		propertyKeyScope:      "",
		propertyKeyTracesURL:  "https://ui.example.com/project/cm0demo/traces",
		propertyKeyTotalCost:  "4.5",
		propertyKeyStale:      "true",
		propertyKeyStaleSince: "2026-09-24T12:00:00Z",
		propertyKeyLastError:  "boom",
	}, cache.degrade(project, "", errors.New("boom"), testTo))
}

// Values measured on another Langfuse project must not pass as this one's.
func TestDegradeDropsCacheFromAnotherSource(t *testing.T) {
	cache := newLastGood()
	cache.record("demo", pluginapi.PropertyMap{
		propertyKeyProjectID: "cm0demo",
		propertyKeyHost:      "https://langfuse.example.com",
		propertyKeyTotalCost: "4.5",
	})
	cache.degrade(testProject("https://langfuse.example.com"), "", errors.New("first"), testFrom)

	moved := testProject("https://langfuse.example.com")
	moved.ProjectID = "cm0other"

	assert.Equal(t, pluginapi.PropertyMap{
		propertyKeyProjectID:  "cm0other",
		propertyKeyHost:       "https://langfuse.example.com",
		propertyKeyScope:      "",
		propertyKeyTracesURL:  "https://langfuse.example.com/project/cm0other/traces",
		propertyKeyStale:      "true",
		propertyKeyStaleSince: "2026-09-24T12:00:00Z",
		propertyKeyLastError:  "second",
	}, cache.degrade(moved, "", errors.New("second"), testTo))
}

// A scope that never succeeded has no cache; stale_since must still hold.
func TestDegradeKeepsStaleSinceWithoutCache(t *testing.T) {
	cache := newLastGood()
	project := testProject("https://langfuse.example.com")

	cache.degrade(project, "", errors.New("first"), testFrom)
	properties := cache.degrade(project, "", errors.New("second"), testTo)

	assert.Equal(t, "2026-09-23T12:00:00Z", properties[propertyKeyStaleSince])
}
