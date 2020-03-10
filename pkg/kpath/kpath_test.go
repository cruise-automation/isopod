package kpath

import (
	"testing"

	_ "github.com/golang/glog"
	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		path string
		want kpath
	}{
		{
			path: "metadata.creationTimestamp",
			want: kpath{
				Part: "metadata",
				Path: "creationTimestamp",
				More: true,
			},
		},
		{
			path: "metadata.annotations[\"isopod.getcruise.com/context\"]",
			want: kpath{
				Part: "metadata",
				Path: "annotations[\"isopod.getcruise.com/context\"]",
				More: true,
			},
		},
		{
			path: "annotations[\"isopod.getcruise.com/context\"]",
			want: kpath{
				Part: "annotations",
				Path: "[\"isopod.getcruise.com/context\"]",
				More: true,
			},
		},
		{
			path: "[\"isopod.getcruise.com/context\"]",
			want: kpath{
				Part: "isopod.getcruise.com/context",
				Path: "",
				More: false,
			},
		},
		{
			path: "array[2]",
			want: kpath{
				Part: "array",
				Path: "[2]",
				More: true,
			},
		},
		{
			path: "[2][\"next\"]",
			want: kpath{
				Part: "2",
				Path: "[\"next\"]",
				More: true,
			},
		},
		{
			path: "[2].next",
			want: kpath{
				Part: "2",
				Path: "next",
				More: true,
			},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {

			got, err := parse(tc.path)
			if err != nil {
				t.Errorf("Unexpected error.\nWant: <nil>\nGot: %q", err)
			}

			if !cmp.Equal(tc.want, got) {
				t.Errorf("Unexpected parse result: \nWant: %+v\nGot: %+v", tc.want, got)
			}

		})
	}
}

func TestSplit(t *testing.T) {
	for _, tc := range []struct {
		path string
		want []string
	}{
		{
			path: "metadata.creationTimestamp",
			want: []string{"metadata", "creationTimestamp"},
		},
		{
			path: "metadata.annotations[\"isopod.getcruise.com/context\"]",
			want: []string{"metadata", "annotations", "isopod.getcruise.com/context"},
		},
		{
			path: "annotations[\"isopod.getcruise.com/context\"]",
			want: []string{"annotations", "isopod.getcruise.com/context"},
		},
		{
			path: "[\"isopod.getcruise.com/context\"]",
			want: []string{"isopod.getcruise.com/context"},
		},
		{
			path: "array[2]",
			want: []string{"array", "2"},
		},
		{
			path: "array[2][\"next\"]",
			want: []string{"array", "2", "next"},
		},
		{
			path: "array[2].next",
			want: []string{"array", "2", "next"},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {

			got, err := Split(tc.path)
			if err != nil {
				t.Errorf("Unexpected error.\nWant: <nil>\nGot: %q", err)
			}

			if !cmp.Equal(tc.want, got) {
				t.Errorf("Unexpected split: \nWant: %v\nGot: %v", tc.want, got)
			}

		})
	}
}
