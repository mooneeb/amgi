package filter

import (
	"strings"
	"testing"

	"github.com/mooneeb/amgi/internal/config"
)

func TestIsFieldsMatch(t *testing.T) {
	ptrTrue := true
	tests := []struct {
		name    string
		fields  []string
		filters *config.FieldOperators
		want    bool
	}{
		{
			name:    "nil filters",
			fields:  []string{"a", "b"},
			filters: nil,
			want:    true,
		},
		{
			name:    "in match",
			fields:  []string{"foo", "bar"},
			filters: &config.FieldOperators{In: []string{"bar"}},
			want:    true,
		},
		{
			name:    "in no match",
			fields:  []string{"foo"},
			filters: &config.FieldOperators{In: []string{"bar"}},
			want:    false,
		},
		{
			name:    "notIn match field in list",
			fields:  []string{"bad"},
			filters: &config.FieldOperators{NotIn: []string{"bad"}},
			want:    false,
		},
		{
			name:    "notIn no match field not in list",
			fields:  []string{"good"},
			filters: &config.FieldOperators{NotIn: []string{"bad"}},
			want:    true,
		},
		{
			name:    "exists true empty fields",
			fields:  nil,
			filters: &config.FieldOperators{Exists: &ptrTrue},
			want:    false,
		},
		{
			name:    "exists true non-empty fields",
			fields:  []string{"x"},
			filters: &config.FieldOperators{Exists: &ptrTrue},
			want:    true,
		},
		{
			name:    "doesNotExist true empty fields",
			fields:  nil,
			filters: &config.FieldOperators{DoesNotExist: &ptrTrue},
			want:    true,
		},
		{
			name:    "doesNotExist true non-empty fields",
			fields:  []string{"x"},
			filters: &config.FieldOperators{DoesNotExist: &ptrTrue},
			want:    false,
		},
		{
			name:   "combined in and exists",
			fields: []string{"wanted"},
			filters: &config.FieldOperators{
				In:     []string{"wanted"},
				Exists: &ptrTrue,
			},
			want: true,
		},
		{
			name:   "combined in and exists fails empty",
			fields: nil,
			filters: &config.FieldOperators{
				In:     []string{"wanted"},
				Exists: &ptrTrue,
			},
			want: false,
		},
		{
			name:   "combined in and exists fails wrong label",
			fields: []string{"other"},
			filters: &config.FieldOperators{
				In:     []string{"wanted"},
				Exists: &ptrTrue,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFieldsMatch(tt.fields, tt.filters)
			if got != tt.want {
				t.Fatalf("isFieldsMatch(%v, %#v) = %v, want %v", tt.fields, tt.filters, got, tt.want)
			}
		})
	}
}

func TestIsFieldMatch(t *testing.T) {
	ptrTrue := true
	tests := []struct {
		name    string
		field   string
		filters *config.FieldOperators
		want    bool
		wantErr string
	}{
		{
			name:    "nil filters",
			field:   "anything",
			filters: nil,
			want:    true,
		},
		{
			name:    "in regex match",
			field:   "hello world",
			filters: &config.FieldOperators{In: []string{`^hello`}},
			want:    true,
		},
		{
			name:    "in regex no match",
			field:   "goodbye",
			filters: &config.FieldOperators{In: []string{`^hello`}},
			want:    false,
		},
		{
			name:    "in invalid regex",
			field:   "x",
			filters: &config.FieldOperators{In: []string{"["}},
			wantErr: "failed to match regex",
		},
		{
			name:    "notIn regex match field matches excluded pattern",
			field:   "blocked",
			filters: &config.FieldOperators{NotIn: []string{`^block`}},
			want:    false,
		},
		{
			name:    "notIn regex no match",
			field:   "allowed",
			filters: &config.FieldOperators{NotIn: []string{`^block`}},
			want:    true,
		},
		{
			name:    "notIn invalid regex",
			field:   "x",
			filters: &config.FieldOperators{NotIn: []string{"("}},
			wantErr: "failed to match regex",
		},
		{
			name:    "exists true empty field",
			field:   "",
			filters: &config.FieldOperators{Exists: &ptrTrue},
			want:    false,
		},
		{
			name:    "exists true non-empty field",
			field:   "user",
			filters: &config.FieldOperators{Exists: &ptrTrue},
			want:    true,
		},
		{
			name:    "doesNotExist true empty field",
			field:   "",
			filters: &config.FieldOperators{DoesNotExist: &ptrTrue},
			want:    true,
		},
		{
			name:    "doesNotExist true non-empty field",
			field:   "user",
			filters: &config.FieldOperators{DoesNotExist: &ptrTrue},
			want:    false,
		},
		{
			name:  "combined in and exists",
			field: "issue-123 fix",
			filters: &config.FieldOperators{
				In:     []string{`fix$`},
				Exists: &ptrTrue,
			},
			want: true,
		},
		{
			name:  "combined in and exists fails empty",
			field: "",
			filters: &config.FieldOperators{
				In:     []string{`.*`},
				Exists: &ptrTrue,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isFieldMatch(tt.field, tt.filters)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("isFieldMatch() err = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("isFieldMatch() err = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("isFieldMatch() unexpected err: %v", err)
			}
			if got != tt.want {
				t.Fatalf("isFieldMatch(%q, %#v) = %v, want %v", tt.field, tt.filters, got, tt.want)
			}
		})
	}
}
