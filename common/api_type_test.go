package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestChannelType2APIType_APImart(t *testing.T) {
	apiType, ok := ChannelType2APIType(constant.ChannelTypeAPImart)
	if !ok {
		t.Fatalf("ChannelType2APIType(APImart) returned ok=false")
	}
	if apiType != constant.APITypeAPImart {
		t.Fatalf("expected APITypeAPImart, got %d", apiType)
	}
}

func TestChannelType2APIType_Fal(t *testing.T) {
	apiType, ok := ChannelType2APIType(constant.ChannelTypeFal)
	if !ok {
		t.Fatalf("ChannelType2APIType(FAL) returned ok=false")
	}
	if apiType != constant.APITypeFal {
		t.Fatalf("expected APITypeFal, got %d", apiType)
	}
}
