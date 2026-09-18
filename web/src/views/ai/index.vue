<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { Promotion } from '@element-plus/icons-vue'
import { aiChat } from '@/api/ai'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

// 会话历史仅保存在内存（刷新即清），后端单轮无状态
const messages = ref<ChatMessage[]>([])
const input = ref('')
const loading = ref(false)
const listRef = ref<HTMLElement>()

const suggestions = [
  '当前哪些 SKU 库存最多？',
  '库存总量和可用总量是多少？',
  'SKU000001 的库存明细',
  '有哪些低可用库存需要补货？',
]

async function scrollToBottom() {
  await nextTick()
  if (listRef.value) listRef.value.scrollTop = listRef.value.scrollHeight
}

async function send(text?: string) {
  const question = (text ?? input.value).trim()
  if (!question || loading.value) return
  input.value = ''
  messages.value.push({ role: 'user', content: question })
  loading.value = true
  await scrollToBottom()
  try {
    const resp = await aiChat(question)
    messages.value.push({ role: 'assistant', content: resp.answer })
  } catch {
    // 错误提示由 request.ts 拦截器统一 ElMessage 弹出，这里仅展示占位气泡
    messages.value.push({ role: 'assistant', content: '回答失败，请稍后重试。' })
  } finally {
    loading.value = false
    await scrollToBottom()
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    void send()
  }
}
</script>

<template>
  <div class="ai-page">
    <el-card shadow="never" class="ai-card">
      <template #header>
        <div class="card-header">
          <span class="title">AI 库存问答</span>
          <span class="subtitle">基于系统实时库存数据的智能助手（检索增强，回答有据可查）</span>
        </div>
      </template>

      <div class="chat-wrap">
        <!-- 消息列表 -->
        <div ref="listRef" class="msg-list">
          <div v-if="messages.length === 0" class="welcome">
            <el-icon :size="42" class="welcome-icon"><ChatDotRound /></el-icon>
            <h3>你好，我是 WMS 库存智能助手</h3>
            <p>我可以基于当前库存数据回答：库存总量、仓库分布、库存 Top SKU、指定 SKU 的库位/批次明细、低可用库存等。</p>
            <div class="suggestions">
              <el-button v-for="s in suggestions" :key="s" plain round size="small" @click="send(s)">
                {{ s }}
              </el-button>
            </div>
          </div>

          <div
            v-for="(m, i) in messages"
            :key="i"
            class="msg-row"
            :class="m.role === 'user' ? 'mine' : 'ai'"
          >
            <div class="avatar">
              <el-icon v-if="m.role === 'assistant'"><ChatDotRound /></el-icon>
              <span v-else>我</span>
            </div>
            <!-- LLM 输出按纯文本渲染（v-text），防止 XSS -->
            <div class="bubble" v-text="m.content"></div>
          </div>

          <!-- 回答生成中 -->
          <div v-if="loading" class="msg-row ai">
            <div class="avatar"><el-icon><ChatDotRound /></el-icon></div>
            <div class="bubble typing">
              <span class="dot"></span><span class="dot"></span><span class="dot"></span>
            </div>
          </div>
        </div>

        <!-- 输入区 -->
        <div class="input-area">
          <el-input
            v-model="input"
            type="textarea"
            :rows="2"
            :maxlength="200"
            show-word-limit
            resize="none"
            placeholder="请输入库存相关问题，如：SKU000002 的库存明细（Enter 发送，Shift+Enter 换行）"
            :disabled="loading"
            @keydown="onKeydown"
          />
          <el-button
            type="primary"
            :icon="Promotion"
            :loading="loading"
            :disabled="!input.trim()"
            class="send-btn"
            @click="send()"
          >
            发送
          </el-button>
        </div>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.ai-page {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.ai-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.ai-card :deep(.el-card__body) {
  flex: 1;
  overflow: hidden;
  padding: 0;
}

.card-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  flex-wrap: wrap;
}

.card-header .title {
  font-weight: 600;
  font-size: 16px;
}

.card-header .subtitle {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.chat-wrap {
  height: 100%;
  display: flex;
  flex-direction: column;
}

/* ---------- 消息列表 ---------- */
.msg-list {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.welcome {
  margin: auto;
  text-align: center;
  color: var(--el-text-color-secondary);
  max-width: 480px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 24px 0;
}

.welcome-icon {
  color: var(--el-color-primary);
}

.welcome h3 {
  margin: 0;
  color: var(--el-text-color-primary);
}

.welcome p {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
}

.suggestions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: center;
  margin-top: 8px;
}

.msg-row {
  display: flex;
  gap: 10px;
  max-width: 88%;
}

.msg-row.mine {
  align-self: flex-end;
  flex-direction: row-reverse;
}

.avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
}

.msg-row.ai .avatar {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}

.msg-row.mine .avatar {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success);
  font-weight: 700;
}

.bubble {
  padding: 10px 14px;
  border-radius: 10px;
  font-size: 14px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

.msg-row.ai .bubble {
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  border-top-left-radius: 2px;
}

.msg-row.mine .bubble {
  background: var(--el-color-primary);
  color: #fff;
  border-top-right-radius: 2px;
}

/* ---------- 输入中动画 ---------- */
.typing {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 14px 16px;
}

.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--el-text-color-secondary);
  animation: blink 1.2s infinite ease-in-out;
}

.dot:nth-child(2) {
  animation-delay: 0.2s;
}

.dot:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes blink {
  0%,
  80%,
  100% {
    opacity: 0.25;
  }
  40% {
    opacity: 1;
  }
}

/* ---------- 输入区 ---------- */
.input-area {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.send-btn {
  height: 54px;
}
</style>
