package main

import (
	"reflect"
	"testing"
)

func TestCenterRectangle(t *testing.T) {
	type args struct {
		outer Rectangle
		inner Rectangle
	}
	tests := []struct {
		name string
		args args
		want Rectangle
	}{
		{
			name: "Inner rectangle is smaller and centered - Case 1",
			args: args{outer: Rectangle{0, 0, 10, 10}, inner: Rectangle{2, 2, 6, 6}},
			want: Rectangle{2, 2, 6, 6},
		},
		{
			name: "Inner rectangle is smaller and centered - Case 2",
			args: args{outer: Rectangle{2, 2, 6, 6}, inner: Rectangle{4, 4, 4, 4}},
			want: Rectangle{2, 2, 6, 6},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CenterRectangle(tt.args.outer, tt.args.inner); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CenterRectangle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_createCompositeImage(t *testing.T) {
	type args struct {
		module         *Module
		rootConfig     Configuration
		currentConfig  Configuration
		parentFileName string
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createCompositeImage(tt.args.module, tt.args.rootConfig, tt.args.currentConfig, tt.args.parentFileName)
		})
	}
}
