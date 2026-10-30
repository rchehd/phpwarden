package psalmconfig

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const namespace = "https://getpsalm.org/schema/config"

type element struct {
	Name     string
	Attrs    []xml.Attr
	Children []*element
	Text     string
}

func clone(node *element) *element {
	copy := &element{Name: node.Name, Text: node.Text, Attrs: append([]xml.Attr{}, node.Attrs...)}
	for _, child := range node.Children {
		copy.Children = append(copy.Children, clone(child))
	}
	return copy
}

func parse(data []byte) (*element, error) {
	if len(data) == 0 || len(data) > 4<<20 {
		return nil, fmt.Errorf("XML must contain 1 byte to 4 MiB")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root *element
	stack := []*element{}
	count := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid XML: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			count++
			if count > 20000 || len(stack) >= 32 || token.Name.Space != "" && token.Name.Space != namespace {
				return nil, fmt.Errorf("unsupported XML namespace or document size/depth")
			}
			node := &element{Name: token.Name.Local}
			seen := map[xml.Name]bool{}
			for _, attr := range token.Attr {
				if seen[attr.Name] {
					return nil, fmt.Errorf("duplicate XML attribute %s", attr.Name.Local)
				}
				seen[attr.Name] = true
				if attr.Name.Local == "xmlns" && attr.Name.Space == "" || attr.Name.Space == "xmlns" {
					continue
				}
				if attr.Name.Space == "http://www.w3.org/2001/XMLSchema-instance" && (attr.Name.Local == "schemaLocation" || attr.Name.Local == "noNamespaceSchemaLocation") {
					continue
				}
				if attr.Name.Space != "" {
					return nil, fmt.Errorf("unsupported XML attribute namespace")
				}
				node.Attrs = append(node.Attrs, attr)
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("expected one XML root")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unmatched XML end element")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(token)) != "" {
					return nil, fmt.Errorf("unexpected XML text")
				}
			} else {
				stack[len(stack)-1].Text += string(token)
			}
		case xml.Directive:
			return nil, fmt.Errorf("XML directives and DTDs are unsupported")
		case xml.ProcInst:
			if token.Target != "xml" || root != nil {
				return nil, fmt.Errorf("XML processing instructions are unsupported")
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("incomplete XML document")
	}
	return root, nil
}

func (node *element) attr(name string) string {
	for _, attr := range node.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (node *element) set(name, value string) {
	for i := range node.Attrs {
		if node.Attrs[i].Name.Local == name {
			node.Attrs[i].Value = value
			return
		}
	}
	node.Attrs = append(node.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func (node *element) attributes(allowed ...string) error {
	for _, attr := range node.Attrs {
		found := false
		for _, name := range allowed {
			found = found || attr.Name.Local == name
		}
		if !found {
			return fmt.Errorf("unsupported %s attribute %s", node.Name, attr.Name.Local)
		}
	}
	return nil
}

func encode(node *element) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString(xml.Header)
	encoder := xml.NewEncoder(&output)
	var write func(*element) error
	write = func(node *element) error {
		start := xml.StartElement{Name: xml.Name{Local: node.Name}, Attr: node.Attrs}
		if err := encoder.EncodeToken(start); err != nil {
			return err
		}
		if len(node.Children) == 0 && node.Text != "" {
			if err := encoder.EncodeToken(xml.CharData(node.Text)); err != nil {
				return err
			}
		}
		for _, child := range node.Children {
			if err := write(child); err != nil {
				return err
			}
		}
		return encoder.EncodeToken(start.End())
	}
	if err := write(node); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
