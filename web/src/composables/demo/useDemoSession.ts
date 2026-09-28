/**
 * 演示会话与租约生命周期（TASK 20）。
 *
 * 负责演示会话的领取、心跳续期、空闲倒计时与释放：页面只消费 remaining / acquiring
 * 等展示状态，不再自己维护 lease 定时器与 token 过期逻辑。
 *
 * 倒计时表示"无操作剩余时间"，只有发生过用户交互才会向后端续期，
 * 挂机期间会话会随后端 TTL 到期自动释放。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'
import { acquireDemoSession, heartbeatDemoSession, releaseDemoSession } from '@/api/demo'
import { ApiError } from '@/api/request'
import { useAuthStore } from '@/stores/auth'

/** 交互事件节流：鼠标移动等高频事件最多每秒记录一次。 */
const ACTIVITY_THROTTLE_MS = 1_000
/** 续期检查周期与两次续期之间的最小间隔。 */
const RENEW_CHECK_INTERVAL_MS = 10_000
const RENEW_MIN_INTERVAL_MS = 20_000
/** 剩余时间低于该阈值（秒）时，用户交互立即触发续期，不等定时器。 */
const RENEW_URGENT_SECONDS = 30
const DEFAULT_SESSION_TTL_SECONDS = 300
const COUNTDOWN_WARNING_SECONDS = 60

const PASSIVE_ACTIVITY_EVENTS = ['mousemove', 'mousedown', 'wheel', 'scroll', 'touchstart'] as const

export interface UseDemoSessionOptions {
  /** 会话被空闲释放或主动退出时调用，用于关闭页面上的演示 UI */
  onSessionClosed?: () => void
}

export function useDemoSession(options: UseDemoSessionOptions = {}) {
  const auth = useAuthStore()
  const router = useRouter()

  const acquiring = ref(false)
  const exiting = ref(false)
  const remaining = ref(0)

  let countdownTimer: number | undefined
  let renewTimer: number | undefined
  let redirectingToLogin = false
  let refreshing = false
  // 最近一次用户交互与最近一次向后端续期的时间戳（毫秒）。
  let lastActivityAt = 0
  let lastRenewAt = 0
  // 有交互后置为 true，等待续期定时器把后端 TTL 同步刷新。
  let renewPending = false
  let countdownWarningShown = false

  const idleTimeoutMinutes = computed(() => Math.max(1, Math.round(idleTTL() / 60)))

  /** 会话空闲时长（秒），来源于后端下发的 TTL。 */
  function idleTTL() {
    return auth.demoSessionExpiresIn || DEFAULT_SESSION_TTL_SECONDS
  }

  function clearTimers() {
    if (countdownTimer !== undefined) window.clearInterval(countdownTimer)
    if (renewTimer !== undefined) window.clearInterval(renewTimer)
    countdownTimer = undefined
    renewTimer = undefined
  }

  /**
   * 记录一次用户交互并标记需要向后端续期。高频事件通过时间戳节流，避免频繁触发。
   * 倒计时不本地乐观重置为完整 TTL，续期成功后按后端返回的真实 TTL 同步。
   */
  function markActivity() {
    if (!auth.isDemo || !auth.demoSessionId) return
    const now = Date.now()
    if (now - lastActivityAt < ACTIVITY_THROTTLE_MS) return
    lastActivityAt = now
    renewPending = true
    if (remaining.value > 0 && remaining.value <= RENEW_URGENT_SECONDS) {
      void refreshSession()
    }
  }

  /**
   * 启动空闲倒计时与续期定时器：
   * - 倒计时每秒递减，归零即自动释放会话；
   * - 只有发生过用户交互时才向后端续期，挂机期间会随 TTL 到期自动释放。
   */
  function startTimers() {
    clearTimers()
    if (remaining.value <= 0) remaining.value = idleTTL()
    lastRenewAt = Date.now()
    renewPending = false
    countdownTimer = window.setInterval(() => {
      if (remaining.value <= 0) {
        void handleIdleTimeout()
        return
      }
      remaining.value -= 1
      if (remaining.value === COUNTDOWN_WARNING_SECONDS && !countdownWarningShown) {
        countdownWarningShown = true
        ElMessage.warning('演示会话将在 1 分钟内空闲释放，可继续操作或点击“重新开始”重置数据')
      }
      if (remaining.value === 0) void handleIdleTimeout()
    }, 1000)
    renewTimer = window.setInterval(() => {
      // 场景执行期间仍需续期，避免长流程进行中会话先过期。
      if (!renewPending || refreshing || !auth.demoSessionId) return
      if (Date.now() - lastRenewAt < RENEW_MIN_INTERVAL_MS) return
      void refreshSession()
    }, RENEW_CHECK_INTERVAL_MS)
  }

  /** 向后端续期演示会话（仅在最近有用户交互时调用）。 */
  async function refreshSession() {
    if (refreshing || !auth.demoSessionId) return
    refreshing = true
    try {
      const info = await heartbeatDemoSession()
      auth.setDemoSession(info)
      remaining.value = info.expires_in
      countdownWarningShown = false
      lastRenewAt = Date.now()
      renewPending = false
    } catch (error) {
      if (error instanceof ApiError && error.code === 70003) {
        if (redirectingToLogin) return
        redirectingToLogin = true
        clearTimers()
        auth.clear()
      }
    } finally {
      refreshing = false
    }
  }

  /**
   * 空闲超时：主动释放会话（释放锁并恢复演示数据），然后回到登录页。
   * 若后端 TTL 已先到期，释放接口会返回失败，忽略即可。
   */
  async function handleIdleTimeout() {
    if (redirectingToLogin) return
    redirectingToLogin = true
    clearTimers()
    try {
      await releaseDemoSession()
    } catch {
      // 后端会话可能已随 TTL 过期，继续清理本地状态。
    }
    auth.clear()
    options.onSessionClosed?.()
    ElMessage.warning('长时间未操作，演示会话已自动释放（数据已恢复初始状态）')
    await router.push('/login')
  }

  async function acquire() {
    if (acquiring.value) return
    acquiring.value = true
    try {
      const info = await acquireDemoSession()
      auth.setDemoSession(info)
      remaining.value = info.expires_in
      countdownWarningShown = false
      startTimers()
    } catch {
      if (redirectingToLogin) return
      redirectingToLogin = true
      auth.clear()
      await router.push('/login')
    } finally {
      acquiring.value = false
    }
  }

  /** 进入演示环境时初始化会话：复用已有会话，否则重新领取。 */
  async function initialize() {
    if (!auth.isDemo) return
    if (auth.demoSessionId) {
      try {
        const info = await heartbeatDemoSession()
        auth.setDemoSession(info)
        remaining.value = info.expires_in
        startTimers()
        return
      } catch {
        auth.clearDemoSession()
        return
      }
    }
    await acquire()
  }

  /** 主动退出演示：释放后端会话并清理本地登录态。 */
  async function release() {
    if (exiting.value) return
    exiting.value = true
    try {
      try {
        await releaseDemoSession()
      } catch {
        // 会话可能已过期，仍需清理本地登录态并返回登录页。
      }
      auth.clear()
      clearTimers()
      options.onSessionClosed?.()
    } finally {
      exiting.value = false
    }
    await router.push('/login')
  }

  onMounted(() => {
    // 用户交互才会刷新空闲倒计时并向后端续期。
    for (const event of PASSIVE_ACTIVITY_EVENTS) {
      window.addEventListener(event, markActivity, { passive: true })
    }
    window.addEventListener('keydown', markActivity)
  })

  onBeforeUnmount(() => {
    clearTimers()
    for (const event of PASSIVE_ACTIVITY_EVENTS) {
      window.removeEventListener(event, markActivity)
    }
    window.removeEventListener('keydown', markActivity)
  })

  return {
    acquiring,
    exiting,
    remaining,
    idleTimeoutMinutes,
    initialize,
    release,
  }
}
