<script setup lang="ts">
const chatStore = useChatStore();
const { conversationId, currentRequestId, input, list, wsStatus, wsData } = storeToRefs(chatStore);

const isSending = computed(() => {
  return Boolean(currentRequestId.value);
});

const hasDraft = computed(() => Boolean(input.value.message.trim()));

const sendable = computed(() => (!hasDraft.value && !isSending.value) || ['CLOSED', 'CONNECTING'].includes(wsStatus.value));

const actionIcon = computed(() => {
  return isSending.value && !hasDraft.value ? 'stop' : 'send';
});

function getMessageByRequestId(requestId?: string) {
  if (!requestId) return null;

  for (let i = list.value.length - 1; i >= 0; i -= 1) {
    const item = list.value[i];
    if (item.role === 'assistant' && item.requestId === requestId) {
      return item;
    }
  }

  return null;
}

watch(wsData, val => {
  if (!val) return;

  const data = JSON.parse(val) as Api.Chat.WsServerMessage;

  if (data.type === 'chat.accepted') {
    conversationId.value = data.conversationId;
    return;
  }

  if (data.type === 'chat.delta') {
    const assistant = getMessageByRequestId(data.requestId);
    if (!assistant) return;

    assistant.status = 'loading';
    assistant.content += data.delta;
    return;
  }

  if (data.type === 'chat.completed') {
    const assistant = getMessageByRequestId(data.requestId);
    if (!assistant) return;

    assistant.finishReason = data.finishReason;
    assistant.status = data.finishReason === 'completed' ? 'finished' : data.finishReason;
    if (currentRequestId.value === data.requestId) {
      currentRequestId.value = '';
    }
    return;
  }

  if (data.type === 'chat.error') {
    const assistant = getMessageByRequestId(data.requestId || currentRequestId.value);
    if (assistant) {
      assistant.status = 'error';
    }
    if (!data.requestId || currentRequestId.value === data.requestId) {
      currentRequestId.value = '';
    }
  }
});

function sendStop() {
  if (!currentRequestId.value) return;

  const payload: Api.Chat.WsClientMessage = {
    type: 'chat.stop',
    requestId: currentRequestId.value
  };
  chatStore.wsSend(JSON.stringify(payload));
}

const handleSend = () => {
  if (isSending.value && !hasDraft.value) {
    sendStop();
    return;
  }

  const message = input.value.message.trim();
  if (!message) return;

  const requestId = crypto.randomUUID();
  currentRequestId.value = requestId;

  list.value.push({
    content: message,
    role: 'user'
  });
  list.value.push({
    content: '',
    role: 'assistant',
    requestId,
    status: 'pending'
  });

  const payload: Api.Chat.WsClientMessage = {
    type: 'chat.send',
    requestId,
    conversationId: conversationId.value || undefined,
    message
  };

  chatStore.wsSend(JSON.stringify(payload));
  input.value.message = '';
};

const inputRef = ref();
// 手动插入换行符（确保所有浏览器兼容）
const insertNewline = () => {
  const textarea = inputRef.value;
  const start = textarea.selectionStart;
  const end = textarea.selectionEnd;

  // 在光标位置插入换行符
  input.value.message = `${input.value.message.substring(0, start)}\n${input.value.message.substring(end)}`;

  // 更新光标位置（在插入的换行符之后）
  nextTick(() => {
    textarea.selectionStart = start + 1;
    textarea.selectionEnd = start + 1;
    textarea.focus(); // 确保保持焦点
  });
};

// ctrl + enter 换行
// enter 发送
const handShortcut = (e: KeyboardEvent) => {
  if (e.key === 'Enter') {
    e.preventDefault();

    if (!e.shiftKey && !e.ctrlKey) {
      handleSend();
    } else insertNewline();
  }
};
</script>

<template>
  <div class="relative w-full b-1 b-#1c1c1c20 bg-#fff p-4 card-wrapper dark:bg-#1c1c1c">
    <textarea
      ref="inputRef"
      v-model.trim="input.message"
      placeholder="给 派聪明 发送消息"
      class="min-h-10 w-full cursor-text resize-none b-none bg-transparent color-#333 caret-[rgb(var(--primary-color))] outline-none dark:color-#f1f1f1"
      @keydown="handShortcut"
    />
    <div class="flex items-center justify-between pt-2">
      <div class="flex items-center text-18px color-gray-500">
        <NText class="text-14px">连接状态：</NText>
        <icon-eos-icons:loading v-if="wsStatus === 'CONNECTING'" class="color-yellow" />
        <icon-fluent:plug-connected-checkmark-20-filled v-else-if="wsStatus === 'OPEN'" class="color-green" />
        <icon-tabler:plug-connected-x v-else class="color-red" />
      </div>
      <NButton :disabled="sendable" strong circle type="primary" @click="handleSend">
        <template #icon>
          <icon-material-symbols:stop-rounded v-if="actionIcon === 'stop'" />
          <icon-guidance:send v-else />
        </template>
      </NButton>
    </div>
  </div>
</template>

<style scoped></style>
