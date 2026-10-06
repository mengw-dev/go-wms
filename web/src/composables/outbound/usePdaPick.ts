import { ref } from 'vue'
import { claimPdaTask, pickPdaTask } from '@/api/outbound'
import type { ApiError } from '@/api/request'
import type { ClaimResult, EntityID, PickParams, PickResult } from '@/api/types'

/**
 * PDA 拣货闭环（领取 → 扫码 → 幂等提交 → 快照刷新）的共享逻辑。
 *
 * 幂等约定：
 * - 每次打开对话框（新的一次拣货动作）调用 `reset()` 清空操作状态；
 * - 领取与拣货各自使用独立的 Idempotency-Key；
 * - key 在首次提交时生成并保存，重试同一次操作时复用，不重复生成——
 *   后端据此回放首次成功结果，避免超时重试造成重复扣减。
 *
 * 快照约定：成功返回的 PickResult 与业务拒绝时 ApiError.data 中的快照，
 * 统一写入 `snapshot`，供界面就地刷新进度。
 */
export function usePdaPick() {
  const claimToken = ref('')
  const leaseExpireAt = ref('')
  const claiming = ref(false)
  const submitting = ref(false)
  /** 最近一次成功/拒绝时后端返回的任务快照（为空表示尚无结果）。 */
  const snapshot = ref<PickResult | null>(null)
  /** 最近一次操作的错误信息（空字符串表示无错误）。 */
  const lastError = ref('')

  // 幂等 key：领取与拣货分开，首次生成后复用。
  const claimKey = ref<string | undefined>(undefined)
  const pickKey = ref<string | undefined>(undefined)

  /** 开启新一轮拣货动作：清空领取状态与操作 key。 */
  function reset() {
    claimToken.value = ''
    leaseExpireAt.value = ''
    claiming.value = false
    submitting.value = false
    snapshot.value = null
    lastError.value = ''
    claimKey.value = undefined
    pickKey.value = undefined
  }

  /** 领取（续领）任务租约；失败时保留 claimKey，便于同一次操作重试。 */
  async function claim(taskId: EntityID): Promise<boolean> {
    if (!claimKey.value) claimKey.value = crypto.randomUUID()
    claiming.value = true
    lastError.value = ''
    try {
      const result: ClaimResult = await claimPdaTask(taskId, claimKey.value)
      claimToken.value = result.claim_token
      leaseExpireAt.value = result.lease_expire_at
      snapshot.value = toPickResult(result)
      return true
    } catch (error) {
      lastError.value = (error as Error).message
      return false
    } finally {
      claiming.value = false
    }
  }

  /** 提交拣货：复用本次操作的 key，成功返回快照；业务拒绝时同样刷新快照并重新抛出。 */
  async function pick(taskId: EntityID, data: Omit<PickParams, 'claim_token'>): Promise<PickResult> {
    if (!pickKey.value) pickKey.value = crypto.randomUUID()
    submitting.value = true
    lastError.value = ''
    try {
      const result = await pickPdaTask(taskId, { ...data, claim_token: claimToken.value }, pickKey.value)
      snapshot.value = result
      return result
    } catch (error) {
      const apiError = error as ApiError
      if (apiError?.data && typeof apiError.data === 'object') {
        snapshot.value = apiError.data as PickResult
      }
      lastError.value = apiError.message
      throw error
    } finally {
      submitting.value = false
    }
  }

  return {
    claimToken,
    leaseExpireAt,
    claiming,
    submitting,
    snapshot,
    lastError,
    reset,
    claim,
    pick,
  }
}

/** 领取响应在 ClaimResult 上展开同一份快照字段，转成 PickResult 供界面统一消费。 */
function toPickResult(result: ClaimResult): PickResult {
  return {
    task_status: result.task_status,
    done_qty: result.done_qty,
    remaining_qty: result.remaining_qty,
    order_status: result.order_status,
  }
}
