package cliresult

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

const (
	SchemaVersion = "2"

	ExitReady               = 0
	ExitGenericError        = 1
	ExitNeedsAuth           = 10
	ExitMisconfigured       = 11
	ExitMissingPrerequisite = 12
	ExitNeedsUserDecision   = 13
)

type Stage struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Current bool   `json:"current,omitempty"`
}

type Warning struct {
	ID          string `json:"id"`
	RequiredFor string `json:"required_for,omitempty"`
	Fix         string `json:"fix,omitempty"`
}

type Diagnostic struct {
	ID         string `json:"id"`
	Field      string `json:"field,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type Action struct {
	Type    string `json:"type"`
	Label   string `json:"label,omitempty"`
	Command string `json:"command,omitempty"`
	Message string `json:"message,omitempty"`
}

type Capabilities struct {
	AuthMethods []string `json:"auth_methods,omitempty"`
}

type NextActionInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Flag     string `json:"flag"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type NextActionAccept struct {
	Method  string            `json:"method"`
	Label   string            `json:"label"`
	Command string            `json:"command"`
	Value   string            `json:"value,omitempty"`
	Inputs  []NextActionInput `json:"inputs,omitempty"`
}

type NextAction struct {
	Type    string             `json:"type"`
	Stage   string             `json:"stage"`
	Command string             `json:"command,omitempty"`
	Reason  string             `json:"reason,omitempty"`
	Accepts []NextActionAccept `json:"accepts,omitempty"`
	Verify  string             `json:"verify,omitempty"`
}

type Envelope struct {
	SchemaVersion string       `json:"schema_version"`
	Ready         bool         `json:"ready"`
	Warehouse     *string      `json:"warehouse"`
	Capabilities  Capabilities `json:"capabilities"`
	Stages        []Stage      `json:"stages"`
	Warnings      []Warning    `json:"warnings,omitempty"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
	NextAction    NextAction   `json:"next_action"`
}

type Status string

const (
	StatusOK      Status = "ok"
	StatusInvalid Status = "invalid"
	StatusError   Status = "error"
)

type Response struct {
	SchemaVersion string       `json:"schema_version"`
	Command       string       `json:"command"`
	Status        Status       `json:"status"`
	Data          any          `json:"data,omitempty"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
	Actions       []Action     `json:"actions,omitempty"`
	ExitCode      int          `json:"-"`
}

func OK(command string, data any) Response {
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        StatusOK,
		Data:          data,
		ExitCode:      ExitReady,
	}
}

func Invalid(command string, data any, diagnostics []Diagnostic) Response {
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        StatusInvalid,
		Data:          data,
		Diagnostics:   diagnostics,
		ExitCode:      ExitMisconfigured,
	}
}

func Error(command string, err error) Response {
	message := "command failed"
	if err != nil && err.Error() != "" {
		message = err.Error()
	}
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        StatusError,
		Diagnostics: []Diagnostic{
			{ID: "error", Message: message},
		},
		ExitCode: ExitGenericError,
	}
}

type HumanPresentable interface {
	HumanDocument() Document
}

type Document struct {
	Title   string
	Summary string
	Blocks  []Block
}

type BlockKind string

const (
	BlockText        BlockKind = "text"
	BlockFields      BlockKind = "fields"
	BlockList        BlockKind = "list"
	BlockTable       BlockKind = "table"
	BlockCode        BlockKind = "code"
	BlockDiagnostics BlockKind = "diagnostics"
	BlockActions     BlockKind = "actions"
)

type Block struct {
	Kind    BlockKind
	Heading string
	Text    string
	Fields  []Field
	Items   []string
	Headers []string
	Rows    [][]string
}

type Field struct {
	Name  string
	Value string
}

type ExitError struct {
	Code int
	Err  error
}

func (err ExitError) Error() string {
	if err.Err == nil {
		return ""
	}
	return err.Err.Error()
}

func WithExitCode(code int, err error) error {
	return ExitError{Code: code, Err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return ExitReady
	}
	if coded, ok := err.(ExitError); ok {
		return coded.Code
	}
	return ExitGenericError
}

func WriteJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

func WriteHuman(out io.Writer, response Response) error {
	return WriteDocument(out, HumanDocument(response))
}

func HumanDocument(response Response) Document {
	if presentable, ok := response.Data.(HumanPresentable); ok {
		return presentable.HumanDocument()
	}

	doc := Document{}
	if response.Status != "" && response.Status != StatusOK {
		doc.Summary = fmt.Sprintf("%s: %s", response.Command, response.Status)
	}
	if len(response.Diagnostics) > 0 {
		doc.Blocks = append(doc.Blocks, diagnosticsBlock(response.Diagnostics))
	}
	if response.Data != nil {
		doc.Blocks = append(doc.Blocks, blockForValue("Data", response.Data))
	}
	if len(response.Actions) > 0 {
		doc.Blocks = append(doc.Blocks, actionsBlock(response.Actions))
	}
	if doc.Summary == "" && len(doc.Blocks) == 0 && response.Command != "" {
		doc.Summary = fmt.Sprintf("%s: %s", response.Command, response.Status)
	}
	return doc
}

func WriteDocument(out io.Writer, doc Document) error {
	writer := &documentWriter{out: out}
	if doc.Title != "" {
		writer.line(doc.Title)
	}
	if doc.Summary != "" {
		writer.line(doc.Summary)
	}
	for _, block := range doc.Blocks {
		if writer.wrote && block.Heading != "" {
			writer.blank()
		}
		writer.block(block)
	}
	return writer.err
}

type documentWriter struct {
	out   io.Writer
	err   error
	wrote bool
}

func (writer *documentWriter) blank() {
	if writer.err != nil {
		return
	}
	_, writer.err = fmt.Fprintln(writer.out)
	writer.wrote = true
}

func (writer *documentWriter) line(text string) {
	if writer.err != nil {
		return
	}
	_, writer.err = fmt.Fprintln(writer.out, text)
	writer.wrote = true
}

func (writer *documentWriter) block(block Block) {
	if block.Heading != "" {
		writer.line(block.Heading)
	}
	switch block.Kind {
	case BlockFields:
		for _, field := range block.Fields {
			writer.line(fmt.Sprintf("%s: %s", field.Name, field.Value))
		}
	case BlockList:
		for _, item := range block.Items {
			writer.line("- " + item)
		}
	case BlockTable:
		writer.table(block.Headers, block.Rows)
	case BlockCode:
		for _, line := range strings.Split(strings.TrimRight(block.Text, "\n"), "\n") {
			writer.line(line)
		}
	case BlockDiagnostics:
		for _, item := range block.Items {
			writer.line("- " + item)
		}
	case BlockActions:
		for _, item := range block.Items {
			writer.line("- " + item)
		}
	default:
		if block.Text != "" {
			writer.line(block.Text)
		}
	}
}

func (writer *documentWriter) table(headers []string, rows [][]string) {
	if len(headers) == 0 {
		for _, row := range rows {
			writer.line(strings.Join(row, "  "))
		}
		return
	}
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = len(header)
	}
	for _, row := range rows {
		for index, value := range row {
			if index < len(widths) && len(value) > widths[index] {
				widths[index] = len(value)
			}
		}
	}
	writer.line(formatTableRow(headers, widths))
	separators := make([]string, len(headers))
	for index, width := range widths {
		separators[index] = strings.Repeat("-", width)
	}
	writer.line(formatTableRow(separators, widths))
	for _, row := range rows {
		writer.line(formatTableRow(row, widths))
	}
}

func formatTableRow(row []string, widths []int) string {
	values := make([]string, len(widths))
	for index := range widths {
		if index < len(row) {
			values[index] = row[index]
		}
		values[index] = values[index] + strings.Repeat(" ", widths[index]-len(values[index]))
	}
	return strings.TrimRight(strings.Join(values, "  "), " ")
}

func diagnosticsBlock(diagnostics []Diagnostic) Block {
	items := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		message := diagnostic.Message
		if diagnostic.Field != "" {
			message = diagnostic.Field + ": " + message
		}
		if diagnostic.Suggestion != "" {
			message += " (" + diagnostic.Suggestion + ")"
		}
		items = append(items, message)
	}
	return Block{Kind: BlockDiagnostics, Heading: "Diagnostics:", Items: items}
}

func actionsBlock(actions []Action) Block {
	items := make([]string, 0, len(actions))
	for _, action := range actions {
		item := action.Label
		if item == "" {
			item = action.Type
		}
		if action.Command != "" {
			item += ": " + action.Command
		}
		if action.Message != "" {
			item += ": " + action.Message
		}
		items = append(items, item)
	}
	return Block{Kind: BlockActions, Heading: "Actions:", Items: items}
}

func blockForValue(heading string, value any) Block {
	if fields, ok := valueFields(value); ok {
		return Block{Kind: BlockFields, Heading: heading + ":", Fields: fields}
	}
	if headers, rows, ok := valueTable(value); ok {
		return Block{Kind: BlockTable, Heading: heading + ":", Headers: headers, Rows: rows}
	}
	if items, ok := valueList(value); ok {
		return Block{Kind: BlockList, Heading: heading + ":", Items: items}
	}
	return Block{Kind: BlockCode, Heading: heading + ":", Text: prettyJSON(value)}
}

func valueFields(value any) ([]Field, bool) {
	reflectValue := indirectValue(reflect.ValueOf(value))
	if !reflectValue.IsValid() {
		return nil, false
	}
	switch reflectValue.Kind() {
	case reflect.Struct:
		reflectType := reflectValue.Type()
		fields := make([]Field, 0, reflectValue.NumField())
		for index := 0; index < reflectValue.NumField(); index++ {
			structField := reflectType.Field(index)
			if structField.PkgPath != "" {
				continue
			}
			fieldValue := indirectValue(reflectValue.Field(index))
			if !isScalar(fieldValue) {
				return nil, false
			}
			name := jsonFieldName(structField)
			if name == "" {
				continue
			}
			fields = append(fields, Field{Name: name, Value: formatScalar(fieldValue)})
		}
		return fields, len(fields) > 0
	case reflect.Map:
		if reflectValue.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		keys := reflectValue.MapKeys()
		names := make([]string, 0, len(keys))
		for _, key := range keys {
			names = append(names, key.String())
			if !isScalar(indirectValue(reflectValue.MapIndex(key))) {
				return nil, false
			}
		}
		sort.Strings(names)
		fields := make([]Field, 0, len(names))
		for _, name := range names {
			fields = append(fields, Field{Name: name, Value: formatScalar(reflectValue.MapIndex(reflect.ValueOf(name)))})
		}
		return fields, len(fields) > 0
	default:
		return nil, false
	}
}

func valueTable(value any) ([]string, [][]string, bool) {
	reflectValue := indirectValue(reflect.ValueOf(value))
	if !reflectValue.IsValid() || reflectValue.Kind() != reflect.Slice && reflectValue.Kind() != reflect.Array || reflectValue.Len() == 0 {
		return nil, nil, false
	}
	first := indirectValue(reflectValue.Index(0))
	if !first.IsValid() || first.Kind() != reflect.Struct {
		return nil, nil, false
	}
	reflectType := first.Type()
	var headers []string
	var fieldIndexes []int
	for index := 0; index < reflectType.NumField(); index++ {
		structField := reflectType.Field(index)
		if structField.PkgPath != "" {
			continue
		}
		fieldValue := indirectValue(first.Field(index))
		if !isScalar(fieldValue) {
			continue
		}
		name := jsonFieldName(structField)
		if name == "" {
			continue
		}
		headers = append(headers, name)
		fieldIndexes = append(fieldIndexes, index)
	}
	if len(headers) == 0 {
		return nil, nil, false
	}
	rows := make([][]string, 0, reflectValue.Len())
	for rowIndex := 0; rowIndex < reflectValue.Len(); rowIndex++ {
		rowValue := indirectValue(reflectValue.Index(rowIndex))
		if !rowValue.IsValid() || rowValue.Kind() != reflect.Struct || rowValue.Type() != reflectType {
			return nil, nil, false
		}
		row := make([]string, 0, len(fieldIndexes))
		for _, fieldIndex := range fieldIndexes {
			row = append(row, formatScalar(indirectValue(rowValue.Field(fieldIndex))))
		}
		rows = append(rows, row)
	}
	return headers, rows, true
}

func valueList(value any) ([]string, bool) {
	reflectValue := indirectValue(reflect.ValueOf(value))
	if !reflectValue.IsValid() || reflectValue.Kind() != reflect.Slice && reflectValue.Kind() != reflect.Array {
		return nil, false
	}
	items := make([]string, 0, reflectValue.Len())
	for index := 0; index < reflectValue.Len(); index++ {
		item := indirectValue(reflectValue.Index(index))
		if !isScalar(item) {
			return nil, false
		}
		items = append(items, formatScalar(item))
	}
	return items, true
}

func indirectValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func isScalar(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	default:
		return false
	}
}

func formatScalar(value reflect.Value) string {
	if !value.IsValid() {
		return ""
	}
	if value.CanInterface() {
		return fmt.Sprint(value.Interface())
	}
	return fmt.Sprint(value)
}

func jsonFieldName(field reflect.StructField) string {
	if tag := field.Tag.Get("json"); tag != "" {
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			return ""
		}
		if name != "" {
			return name
		}
	}
	return strings.ToLower(field.Name[:1]) + field.Name[1:]
}

func prettyJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}
