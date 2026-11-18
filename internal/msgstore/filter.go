package msgstore

import (
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// A group sender-key distribution can arrive with the same stanza ID as the
// actual message. It carries no chat content and must not occupy that store key.
// Messages with content alongside the distribution are still accepted.
func senderKeyOnly(msg *waE2E.Message) bool {
	if msg.GetSenderKeyDistributionMessage() == nil {
		return false
	}
	onlyKeys := true
	msg.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if field.Name() != "senderKeyDistributionMessage" && field.Name() != "messageContextInfo" {
			onlyKeys = false
			return false
		}
		return true
	})
	return onlyKeys
}
