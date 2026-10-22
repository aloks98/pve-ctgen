package configutil

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
)

func Decode(data []byte, target any) error {
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("only one YAML document is allowed")
	}
	return nil
}
