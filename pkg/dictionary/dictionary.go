package dictionary

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)
// Clusters - information about each cluster in dictionary.json
type Cluster struct {
	Token      string `json:"token"`
	Address    string `json:"address"`
	FacName    string `json:"name"`
	FacilityID string `json:"id"`
}

type Table struct {
	Name string `json:"name"`
}

// Dictionary - represent the information from dictionary.json
type Dictionary struct {
	Clusters    []*Cluster `json:"clusters"`
	Tables      []*Table   `json:"tables"`
	TableFields map[string]string
	FieldTypes  map[string]string
}

// NewDictionary - create a Dictionary
func NewDictionary(root string) (*Dictionary, error) {

	results := Dictionary{
		TableFields: map[string]string{},
		FieldTypes:  map[string]string{},
	}

	b, err := os.ReadFile(root + "dictionary.json")
	if err != nil {
		return nil, fmt.Errorf("dictionary - %v", err)
	}
	err = json.Unmarshal(b, &results)
	if err != nil {
		return nil, fmt.Errorf("dictionary unmarshal - %v", err)
	}
	return &results, nil
}

func TrimSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		s = s[:len(s)-len(suffix)]
	}
	return s
}

func (*Dictionary) loadSchema(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// ParseSchema - parse the sqlite schema into the TableFields and FieldTypes structs
func (dictio *Dictionary) ParseSchema(path string) error {

	lines, err := dictio.loadSchema(path)
	if err != nil {
		return err
	}
	table := ""
	fieldlist := ""
	for _, line0 := range lines {
		line := strings.TrimSpace(line0)
		words := strings.Fields(line)
		if len(words) < 2 {
			continue
		}
		if words[0] == ");" {
			continue
		}

		if words[0] == "CREATE" {
			if table != "" {
				dictio.TableFields[table] = fieldlist
			}
			table = words[2]
			fieldlist = ""
		} else {
			w := strings.ReplaceAll(words[0], "`", "")
			ftype := TrimSuffix(words[1], ",")
			if fieldlist != "" {
				fieldlist += ","
			}
			if w == "group" || w == "default" {
				fieldlist += fmt.Sprintf(`"%s"`, w)
			} else {
				fieldlist += w
			}

			dictio.FieldTypes[table+":"+w] = ftype
		}
	}
	return nil

}
