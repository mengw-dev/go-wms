import axios, { type AxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'
import { useAuthStore } from '@/stores/auth'
import type { ApiResponse } from './types'

/**
 * 错误分类，便于调用方按类型分支处理，而不用去匹配提示文案：
 * - auth：登录态失效（HTTP 401）
 * - demo：演示会话相关错误（70002 / 70003 / 70005 / 70006）
 * - business：服务端返回了业务错误码（HTTP 层有响应但业务码非 0）
 * - network：没有拿到响应（断网、超时等）
 */
export type ApiErrorKind = 'auth' | 'demo' | 'business' | 'network'

export class ApiError extends Error {
  code?: number
  status?: number
  data?: unknown
  kind: ApiErrorKind

  constructor(message: string, code?: number, status?: number, data?: unknown, kind: ApiErrorKind = 'business') {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.data = data
    this.kind = kind
  }
}

/**
 * 单个请求的附加选项。
 *
 * 请求按用途分为三类，错误提示策略不同：
 * 1. 用户操作请求（保存/审核/提交/删除）——默认提示，失败必须让用户看到；
 * 2. 后台自动刷新请求——传 `silentError: true`，失败只抛错不打扰用户；
 * 3. 轮询请求（导入任务状态等）——同样传 `silentError: true`，由轮询逻辑决定何时提示。
 *
 * 认证失败（401）与演示会话失效（70003）由拦截器统一处理，不受此选项影响。
 */
export interface RequestOptions {
  silentError?: boolean
}

declare module 'axios' {
  interface AxiosRequestConfig {
    silentError?: boolean
  }
}

/** 判断响应体是否是后端统一结构（非统一结构时原样返回，例如第三方接口）。 */
function isApiResponseBody(value: unknown): value is ApiResponse {
  return !!value && typeof value === 'object' && typeof (value as { code?: unknown }).code === 'number'
}

/** 按 HTTP 状态码与业务码归类错误。 */
function classifyError(status: number | undefined, code: number | undefined): ApiErrorKind {
  if (status === undefined) return 'network'
  if (status === 401) return 'auth'
  if (code === 70002 || code === 70003 || code === 70005 || code === 70006) return 'demo'
  return 'business'
}

// 演示会话失效时只跳转一次，避免并发请求刷屏。
let demoSessionRedirecting = false

const service = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 20000,
})

/**
 * 拦截器在 onFulfilled 中返回 body.data（而非 AxiosResponse），
 * 因此 axios 实例方法的实际返回类型是 Promise<T> 而非 Promise<AxiosResponse<T>>，
 * 静态类型与运行时不一致。这里集中做一次类型转换，
 * 使 get/post/put/del/upload 都能声明准确的返回类型，
 * 不必在每个函数里重复 `as unknown as Promise<T>`。
 */
interface ApiClient {
  get<T>(url: string, config?: AxiosRequestConfig): Promise<T>
  post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T>
  put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T>
  delete<T>(url: string, config?: AxiosRequestConfig): Promise<T>
}

const client = service as unknown as ApiClient

/**
 * 处理演示会话失效（70003）：清空登录态并跳转登录页，全程只执行一次，
 * 防止自动刷新等并发请求产生大量重复提示。
 */
function handleDemoSessionExpired() {
  if (demoSessionRedirecting) return
  demoSessionRedirecting = true
  useAuthStore().clear()
  ElMessage.warning('演示会话已失效，请重新登录')
  if (router.currentRoute.value.path !== '/login') {
    void router.push('/login')
  }
  // 留一个短暂窗口，便于用户重新登录后再次触发。
  window.setTimeout(() => {
    demoSessionRedirecting = false
  }, 1500)
}

// 请求拦截器：注入 token
service.interceptors.request.use((config) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  if (auth.isDemo && auth.demoSessionId) {
    config.headers['X-Demo-Session'] = auth.demoSessionId
  }
  return config
})

// 响应拦截器：统一处理业务码 / 401
service.interceptors.response.use(
  (response) => {
    if (response.config.responseType === 'blob') {
      return response.data
    }
    const body = response.data
    if (isApiResponseBody(body)) {
      if (body.code !== 0) {
        const msg = body.msg || '操作失败'
        if (!response.config.silentError) ElMessage.error(msg)
        return Promise.reject(new ApiError(msg, body.code, undefined, body.data, 'business'))
      }
      return body.data
    }
    return body
  },
  (error) => {
    const status = error?.response?.status
    const code = error?.response?.data?.code
    // 后台自动刷新与轮询等场景可在请求级声明静默，失败时只抛错不提示。
    const silent = error?.config?.silentError === true
    if (status === 401) {
      useAuthStore().clear()
      if (router.currentRoute.value.path !== '/login') {
        ElMessage.error('登录已失效，请重新登录')
        router.push('/login')
      }
    } else if (code === 70005 || code === 70006) {
      // 这两类演示错误由演示中心给出下一步操作对话框，避免同时出现重复提示。
    } else if (status === 423 && code === 70003) {
      // 演示会话失效：清空登录态并跳转登录页，防刷屏。
      handleDemoSessionExpired()
    } else if (status === 423 && code === 70002) {
      if (!silent) {
        ElMessage.warning(error?.response?.data?.msg || '演示环境正在被其他访客使用，对方空闲约 5 分钟后自动释放，请稍后重试')
      }
    } else {
      const msg = error?.response?.data?.msg || error?.message || '网络异常'
      if (!silent) ElMessage.error(msg)
    }
    return Promise.reject(
      new ApiError(
        error?.response?.data?.msg || error?.message || '网络异常',
        error?.response?.data?.code,
        status,
        error?.response?.data?.data,
        classifyError(status, code),
      ),
    )
  },
)

export function get<T = unknown>(
  url: string,
  params?: Record<string, unknown>,
  options?: RequestOptions,
): Promise<T> {
  return client.get<T>(url, { params, ...options })
}

export function post<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
  return client.post<T>(url, data, config)
}

export function put<T = unknown>(url: string, data?: unknown): Promise<T> {
  return client.put<T>(url, data)
}

export function del<T = unknown>(url: string): Promise<T> {
  return client.delete<T>(url)
}

export function upload<T = unknown>(url: string, formData: FormData): Promise<T> {
  return client.post<T>(url, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}
