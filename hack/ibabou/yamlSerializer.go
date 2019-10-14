package ibabou

import (
	"bytes"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// YamlSerializer serializes into yaml
type YamlSerializer struct {
	s *json.Serializer
}

// NewSerializer creates a new default yaml serializer
func NewSerializer() *YamlSerializer {
	return &YamlSerializer{
		s: json.NewYAMLSerializer(json.DefaultMetaFactory, nil, nil),
	}
}

// Serialize the objects
func (y *YamlSerializer) Serialize(obj runtime.Object) string {
	buff := bytes.NewBufferString("")
	if err := y.s.Encode(obj, buff); err != nil {
		return ""
	}

	return buff.String()
}
