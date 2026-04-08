<template>
  <div class="messages-page card">
    <div class="messages-sidebar">
      <div class="messages-title">消息</div>
      <div v-if="conversationLoading" class="placeholder">加载中...</div>
      <div v-else-if="conversations.length === 0" class="placeholder">暂无会话</div>
      <div v-else class="conversation-list">
        <div
          v-for="item in conversations"
          :key="item.target_id"
          class="conversation-item"
          :class="{ active: selectedTargetId === item.target_id }"
          @click="selectConversation(item)"
        >
          <el-avatar :size="42" :src="item.user?.avatar || ''" class="clickable-avatar" @click.stop="goToUserProfile(item.user?.id)">{{ item.user?.nickname?.charAt(0) || 'U' }}</el-avatar>
          <div class="conversation-meta">
            <div class="top-row">
              <span class="name">{{ item.user?.nickname || '用户' }}</span>
              <span class="time">{{ formatTime(item.last_time) }}</span>
            </div>
            <div class="last-msg">{{ item.last_msg || '暂无消息' }}</div>
          </div>
        </div>
      </div>
    </div>

    <div class="messages-main">
      <template v-if="selectedTargetId">
        <div class="chat-header">{{ selectedUserName }}</div>
        <div class="chat-list" ref="chatListRef">
          <div
            v-for="msg in messages"
            :key="msg.id"
            class="chat-item"
            :class="{ mine: msg.from_user_id === currentUserId }"
          >
            <template v-if="msg.from_user_id === currentUserId">
              <div class="bubble">{{ msg.content }}</div>
              <el-avatar :size="30" :src="currentUser?.avatar || ''" class="clickable-avatar" @click="goToUserProfile(currentUserId)">
                {{ currentUser?.nickname?.charAt(0) || '我' }}
              </el-avatar>
            </template>
            <template v-else>
              <el-avatar :size="30" :src="msg.from_user?.avatar || ''" class="clickable-avatar" @click="goToUserProfile(msg.from_user?.id)">
                {{ msg.from_user?.nickname?.charAt(0) || 'U' }}
              </el-avatar>
              <div class="bubble">{{ msg.content }}</div>
            </template>
          </div>
        </div>

        <div class="composer">
          <el-input
            v-model="inputContent"
            type="textarea"
            :rows="2"
            maxlength="1000"
            placeholder="请输入消息..."
            @keyup.enter.exact.prevent="send"
          />
          <el-button type="primary" :loading="sending" :disabled="!inputContent.trim()" @click="send">发送</el-button>
        </div>
      </template>

      <div v-else class="no-chat">选择一个联系人开始聊天</div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { messageApi } from '../api'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()

const conversations = ref([])
const conversationLoading = ref(false)
const selectedTargetId = ref(0)
const selectedUserName = ref('')

const messages = ref([])
const inputContent = ref('')
const sending = ref(false)
const chatListRef = ref(null)

const currentUser = computed(() => JSON.parse(sessionStorage.getItem('user') || 'null'))

const currentUserId = computed(() => currentUser.value?.id || 0)

onMounted(async () => {
  await loadConversations()

  const target = Number(route.query.target || 0)
  if (target) {
    selectedTargetId.value = target
    const userName = route.query.name ? String(route.query.name) : '私信'
    selectedUserName.value = userName
    await loadMessages(target)
  }
})

async function loadConversations() {
  conversationLoading.value = true
  try {
    const res = await messageApi.getConversations(1, 50)
    conversations.value = res.data.list || []

    if (!selectedTargetId.value && conversations.value.length > 0) {
      selectConversation(conversations.value[0])
    }
  } catch (e) {
    // handled by interceptor
  } finally {
    conversationLoading.value = false
  }
}

async function loadMessages(targetId) {
  try {
    const res = await messageApi.getMessages(targetId, 1, 100)
    messages.value = (res.data.list || []).slice().reverse()
    await nextTick()
    scrollToBottom()
  } catch (e) {
    // handled by interceptor
  }
}

function selectConversation(item) {
  selectedTargetId.value = item.target_id
  selectedUserName.value = item.user?.nickname || '私信'
  router.replace({ path: '/messages', query: { target: item.target_id, name: selectedUserName.value } })
  loadMessages(item.target_id)
}

async function send() {
  const content = inputContent.value.trim()
  if (!content || !selectedTargetId.value) return

  sending.value = true
  try {
    await messageApi.sendMessage(selectedTargetId.value, content)
    inputContent.value = ''
    await loadMessages(selectedTargetId.value)
    await loadConversations()
  } catch (e) {
    ElMessage.error('发送失败')
  } finally {
    sending.value = false
  }
}

function scrollToBottom() {
  const el = chatListRef.value
  if (!el) return
  el.scrollTop = el.scrollHeight
}

function goToUserProfile(userId) {
  if (!userId) return
  router.push(`/profile/${userId}`)
}

function formatTime(timeStr) {
  if (!timeStr) return ''
  const date = new Date(timeStr)
  if (Number.isNaN(date.getTime())) return ''
  const hh = String(date.getHours()).padStart(2, '0')
  const mm = String(date.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}
</script>

<style scoped>
.messages-page {
  display: grid;
  grid-template-columns: 280px 1fr;
  gap: 0;
  padding: 0;
  min-height: 70vh;
  overflow: hidden;
}

.messages-sidebar {
  border-right: 1px solid rgba(120, 130, 170, 0.16);
  background: #f8faff;
}

.messages-title {
  padding: 14px 16px;
  font-size: 18px;
  font-weight: 700;
  border-bottom: 1px solid rgba(120, 130, 170, 0.16);
}

.placeholder {
  padding: 20px 16px;
  color: #96a0b5;
}

.conversation-list {
  max-height: calc(70vh - 52px);
  overflow-y: auto;
}

.conversation-item {
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  cursor: pointer;
  border-bottom: 1px solid rgba(120, 130, 170, 0.08);
}

.clickable-avatar {
  cursor: pointer;
}

.conversation-item.active {
  background: #edf3ff;
}

.conversation-meta {
  min-width: 0;
  flex: 1;
}

.top-row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}

.name {
  font-size: 14px;
  font-weight: 600;
}

.time {
  font-size: 12px;
  color: #9aa4b8;
}

.last-msg {
  margin-top: 4px;
  font-size: 12px;
  color: #7f899d;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.messages-main {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.chat-header {
  height: 52px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  font-weight: 600;
  border-bottom: 1px solid rgba(120, 130, 170, 0.16);
}

.chat-list {
  flex: 1;
  overflow-y: auto;
  padding: 14px 16px;
  background: #f7f9fd;
}

.chat-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 10px;
}

.chat-item.mine {
  justify-content: flex-end;
}

.bubble {
  max-width: 62%;
  background: #fff;
  border: 1px solid rgba(120, 130, 170, 0.15);
  padding: 8px 10px;
  border-radius: 6px;
  line-height: 1.6;
  font-size: 13px;
  color: #2a3146;
}

.chat-item.mine .bubble {
  background: #e7efff;
  border-color: rgba(118, 146, 226, 0.24);
}

.composer {
  padding: 12px;
  border-top: 1px solid rgba(120, 130, 170, 0.16);
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10px;
  align-items: end;
}

.no-chat {
  margin: auto;
  color: #9aa3b6;
}
</style>
