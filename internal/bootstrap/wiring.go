package bootstrap

import (
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/scheduledtasks"
)

// wireChannelGateway shares the connected channel gateway with services that
// need to send or receive channel messages.
func wireChannelGateway(
	chatService *chat.ChatService,
	scheduledTasksService *scheduledtasks.ScheduledTasksService,
	channelGateway *channels.Gateway,
) {
	if chatService != nil {
		chatService.SetGateway(channelGateway)
	}
	if scheduledTasksService != nil {
		scheduledTasksService.SetNotificationGateway(channelGateway)
	}
}
