package server

import (
	"testing"
)

func TestBase_BeforeCreate(t *testing.T) {
	t.Run("check BeforeCreate creates valid ID", func(t *testing.T) {
		// instantiate a subclass of the base struct, check that the uuid is initialized to something non-nil

		base := &Base{}
		if !base.ID.IsNil() {
			t.Errorf("base instance has non-nil ID, expected nil %v", base.ID)
		}
		if err := base.BeforeCreate(nil); err != nil {
			t.Errorf("BeforeCreate() error = %v", err)
		}
		if base.ID.IsNil() {
			t.Errorf("base instance has nil ID after BeforeCreate, expected not nil %v", base.ID)
		}
	})
}
