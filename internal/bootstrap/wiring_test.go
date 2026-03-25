package bootstrap

import (
	"reflect"
	"testing"

	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/scheduledtasks"
)

func TestWireChannelGatewayInjectsScheduledTaskNotificationGateway(t *testing.T) {
	channelGateway := channels.NewGateway(nil, nil)
	chatService := chat.NewChatService(nil)
	scheduledTasksService := scheduledtasks.NewScheduledTasksService(nil, nil, nil)

	wireChannelGateway(chatService, scheduledTasksService, channelGateway)

	chatGatewayField := reflect.ValueOf(chatService).Elem().FieldByName("gateway")
	if chatGatewayField.IsNil() {
		t.Fatal("expected chat service gateway to be injected")
	}

	notificationGatewayField := reflect.ValueOf(scheduledTasksService).Elem().FieldByName("notificationGateway")
	if notificationGatewayField.IsNil() {
		t.Fatal("expected scheduled task notification gateway to be injected")
	}
}
