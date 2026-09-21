package anylistpb

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestProto2FieldNumbersAndPresence(t *testing.T) {
	// Hand-encoded ListItem: identifier=1, name=4, checked=6 (explicit false),
	// deprecatedQuantity=18, quantityPb=21 with rawQuantity=3 (explicit empty).
	wire := []byte{0x0a, 1, 'i', 0x22, 1, 'n', 0x30, 0, 0x92, 1, 0, 0xaa, 1, 2, 0x1a, 0}
	var item ListItem
	if err := proto.Unmarshal(wire, &item); err != nil {
		t.Fatal(err)
	}
	if item.GetIdentifier() != "i" || item.GetName() != "n" || item.Checked == nil || *item.Checked || item.DeprecatedQuantity == nil || item.QuantityPb == nil || item.QuantityPb.RawQuantity == nil {
		t.Fatalf("field number or presence mismatch: %v", &item)
	}
	encoded, err := proto.Marshal(&item)
	if err != nil {
		t.Fatal(err)
	}
	var again ListItem
	if err := proto.Unmarshal(encoded, &again); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&item, &again) {
		t.Fatal("round trip changed proto2 presence")
	}
	if err := proto.Unmarshal([]byte{0x22, 1, 'n'}, &again); err == nil {
		t.Fatal("missing required identifier accepted")
	}
}

func TestUnknownFieldsSurviveClone(t *testing.T) {
	// Item field 30 (productUpc) is deliberately outside the subset schema.
	wire := []byte{0x0a, 1, 'i', 0xf2, 1, 3, 'u', 'p', 'c'}
	var item ListItem
	if err := proto.Unmarshal(wire, &item); err != nil {
		t.Fatal(err)
	}
	cloned := proto.Clone(&item).(*ListItem)
	cloned.Name = proto.String("updated")
	encoded, err := proto.Marshal(cloned)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, wire[3:]) {
		t.Fatal("unknown product field was discarded")
	}
}

func TestEditOperationAcknowledgmentFieldNumber(t *testing.T) {
	// processedOperations is repeated string field 3.
	wire := []byte{0x1a, 1, 'a', 0x1a, 1, 'b'}
	var response PBEditOperationResponse
	if err := proto.Unmarshal(wire, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.ProcessedOperations) != 2 || response.ProcessedOperations[0] != "a" || response.ProcessedOperations[1] != "b" {
		t.Fatal("incorrect acknowledgment field number or cardinality")
	}
}
