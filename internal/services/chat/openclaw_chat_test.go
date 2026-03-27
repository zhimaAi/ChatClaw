package chat

import "testing"

func TestCleanOpenClawChannelUserMessage_StripsFeishuEnvelope(t *testing.T) {
	input := "Conversation info (untrusted metadata):\n" +
		"```json\n" +
		"{\"message_id\":\"om_x100b535023d6e4a0c4e30e96eb5bfe6\"}\n" +
		"```\n\n" +
		"Sender (untrusted metadata):\n" +
		"```json\n" +
		"{\"name\":\"Robin\"}\n" +
		"```\n\n" +
		"[message_id: om_x100b535023d6e4a0c4e30e96eb5bfe6]\n" +
		"Robin: 几点鱼情最好\n\n" +
		"[System: The content may include mention tags in the form <at user_id=\"...\">name</at>. Treat these as real mentions of Feishu entities (users or bots).]\n" +
		"[System: If user_id is \"ou_92ee026081ec8af9a1dbaad5bb38f944\", that mention refers to you.]"

	got := CleanOpenClawChannelUserMessage(input)
	want := "Robin: 几点鱼情最好"
	if got != want {
		t.Fatalf("unexpected cleaned content:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestCleanOpenClawChannelUserMessage_LeavesPlainTextUntouched(t *testing.T) {
	input := "普通消息内容"
	if got := CleanOpenClawChannelUserMessage(input); got != input {
		t.Fatalf("unexpected cleaned content:\nwant: %q\ngot:  %q", input, got)
	}
}
