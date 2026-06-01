package devices

import (
	"context"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

type contactImportDisabledStore struct {
	next store.ContactStore
}

type contactImportDisabledContainer struct {
	next store.DeviceContainer
}

func disableContactImportForDevice(dev *store.Device) {
	if dev == nil {
		return
	}
	dev.Contacts = disableContactImport(dev.Contacts)
	dev.Container = disableContactImportContainer(dev.Container)
}

func disableContactImport(next store.ContactStore) store.ContactStore {
	switch next.(type) {
	case nil, contactImportDisabledStore:
		return next
	}
	return contactImportDisabledStore{next: next}
}

func disableContactImportContainer(next store.DeviceContainer) store.DeviceContainer {
	switch next.(type) {
	case nil, contactImportDisabledContainer:
		return next
	}
	return contactImportDisabledContainer{next: next}
}

func (c contactImportDisabledContainer) PutDevice(ctx context.Context, dev *store.Device) error {
	if c.next == nil {
		disableContactImportForDevice(dev)
		return nil
	}
	err := c.next.PutDevice(ctx, dev)
	disableContactImportForDevice(dev)
	return err
}

func (c contactImportDisabledContainer) DeleteDevice(ctx context.Context, dev *store.Device) error {
	if c.next == nil {
		return nil
	}
	return c.next.DeleteDevice(ctx, dev)
}

func (s contactImportDisabledStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	return false, "", nil
}

func (s contactImportDisabledStore) PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error) {
	return false, "", nil
}

func (s contactImportDisabledStore) PutContactName(context.Context, types.JID, string, string) error {
	return nil
}

func (s contactImportDisabledStore) PutAllContactNames(context.Context, []store.ContactEntry) error {
	return nil
}

func (s contactImportDisabledStore) PutManyRedactedPhones(ctx context.Context, entries []store.RedactedPhoneEntry) error {
	return nil
}

func (s contactImportDisabledStore) GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error) {
	if s.next == nil {
		return types.ContactInfo{}, nil
	}
	return s.next.GetContact(ctx, user)
}

func (s contactImportDisabledStore) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	if s.next == nil {
		return map[types.JID]types.ContactInfo{}, nil
	}
	return s.next.GetAllContacts(ctx)
}
