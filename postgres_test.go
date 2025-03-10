package postgres

import (
	"testing"
"gorm.io/gorm/schema"
)

func Test_DataTypeOf(t *testing.T) {
	type fields struct {
		Config *Config
	}
	type args struct {
		field *schema.Field
	}
	tests := []struct {
		name string
		fields fields
		args args
		want string
	} {
		{
			name: "it should return boolean",
			args: args{field: &schema.Field{DataType: schema.Bool}},
			want: "boolean",
		},
		{
			name: "it should return text -1",
			args: args{field: &schema.Field{DataType: schema.String, Size: -1}},
			want: "text",
		},
		{
			name: "it should return text > 10485760",
			args: args{field: &schema.Field{DataType: schema.String, Size: 12345678}},
			want: "text",
		},
		{
			name: "it should return varchar(100)",
			args: args{field: &schema.Field{DataType: schema.String, Size: 100}},
			want: "varchar(100)",
		},
		{
			name: "it should return varchar(10485760)",
			args: args{field: &schema.Field{DataType: schema.String, Size: 10485760}},
			want: "varchar(10485760)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialector := Dialector{
				Config: tt.fields.Config,
			}
			if got := dialector.DataTypeOf(tt.args.field); got != tt.want {
				t.Errorf("DataTypeOf() = %v, want %v", got, tt.want)
			}
		})
	}
}