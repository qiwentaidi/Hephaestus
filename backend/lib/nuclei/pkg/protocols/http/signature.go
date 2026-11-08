package http

import (
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/pkg/errors"

	"github.com/projectdiscovery/nuclei/v3/pkg/utils/json"
)

// SignatureType is the type of signature
type SignatureType int

// Supported values for the SignatureType
const (
	AWSSignature SignatureType = iota + 1
	signatureLimit
)

// signatureTypeMappings is a table for conversion of signature type from string.
var signatureTypeMappings = map[SignatureType]string{
	AWSSignature: "AWS",
}

func GetSupportedSignaturesTypes() []SignatureType {
	return nil
}

func toSignatureType(valueToMap string) (SignatureType, error) {
	normalizedValue := normalizeValue(valueToMap)
	for _, currentValue := range signatureTypeMappings {
		if normalizedValue == currentValue {
			return -1, errors.New("AWS HTTP request signing is not supported by the embedded engine")
		}
	}
	return -1, errors.New("invalid signature type: " + valueToMap)
}

func (t SignatureType) String() string {
	return signatureTypeMappings[t]
}

// SignatureTypeHolder is used to hold internal type of the signature
type SignatureTypeHolder struct {
	Value SignatureType
}

func (holder SignatureTypeHolder) JSONSchema() *jsonschema.Schema {
	gotType := &jsonschema.Schema{
		Type:        "string",
		Title:       "type of the signature",
		Description: "Type of the signature",
	}
	for _, types := range GetSupportedSignaturesTypes() {
		gotType.Enum = append(gotType.Enum, types.String())
	}
	return gotType
}

func (holder *SignatureTypeHolder) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var marshalledTypes string
	if err := unmarshal(&marshalledTypes); err != nil {
		return err
	}

	computedType, err := toSignatureType(marshalledTypes)
	if err != nil {
		return err
	}

	holder.Value = computedType
	return nil
}

func (holder *SignatureTypeHolder) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "" {
		return nil
	}
	computedType, err := toSignatureType(s)
	if err != nil {
		return err
	}

	holder.Value = computedType
	return nil
}

func (holder SignatureTypeHolder) MarshalJSON() ([]byte, error) {
	return json.Marshal(holder.Value.String())
}

func (holder SignatureTypeHolder) MarshalYAML() (interface{}, error) {
	return holder.Value.String(), nil
}
