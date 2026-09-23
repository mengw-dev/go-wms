import axios, { type AxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'
import { useAuthStore } from '@/stores/auth'

export class ApiError extends Error {
  code?: number
  status?: number
  data?: unknown

  constructor(message: string, code?: number, status?: number, data?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.data = data
  }
}

// 演示会话失效时只跳转一次，避免并发请求刷屏。
let demoSessionRedirecting = false

const service = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 20000,
})

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
    if (body && typeof body === 'object' && typeof body.code === 'number') {
      if (body.code !== 0) {
        const msg = body.msg || '操作失败'
        ElMessage.error(msg)
        return Promise.reject(new ApiError(msg, body.code, undefined, body.data))
      }
      return body.data
    }
    return body
  },
  (error) => {
    const status = error?.response?.status
    const code = error?.response?.data?.code
    if (status === 401) {
      useAuthStore().clear()
      if (router.currentRoute.value.path !== '/login') {
        ElMessage.error('登录已失效，请重新登录')
        router.push('/login')
      }
    } else if (code === 70005 || code === 70006) {
      // 这两类演示错误由业务流程中心给出下一步操作对话框，避免同时出现重复提示。
    } else if (status === 423 && code === 70003) {
      // 演示会话失效：清空登录态并跳转登录页，防刷屏。
      handleDemoSessionExpired()
    } else if (status === 423 && code === 70002) {
      ElMessage.warning(error?.response?.data?.msg || '演示环境正在被其他访客使用，对方空闲约 5 分钟后自动释放，请稍后重试')
    } else {
      const msg = error?.response?.data?.msg || error?.message || '网络异常'
      ElMessage.error(msg)
    }
    return Promise.reject(
      new ApiError(
        error?.response?.data?.msg || error?.message || '网络异常',
        error?.response?.data?.code,
        status,
        error?.response?.data?.data,
      ),
    )
  },
)

// 注意：拦截器在 onFulfilled 中返回 body.data（而非 AxiosResponse），
// 因此 service.get/post 等方法的实际返回类型是 Promise<T> 而非 Promise<AxiosResponse>。
// TypeScript 静态类型仍按 AxiosResponse 推断，所以这里用 `as unknown as Promise<T>`
// 跳过结构类型检查。这是 axios 拦截器返回非标准类型的常见妥协，不应简化。
// 调用方使用 get<T>/post<T> 时无需再断言，类型安全已得到保证。
export function get<T = unknown>(url: string, params?: Record<string, unknown>): Promise<T> {
  return service.get(url, { params }) as unknown as Promise<T>
}

export function post<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
  return service.post(url, data, config) as unknown as Promise<T>
}

export function put<T = unknown>(url: string, data?: unknown): Promise<T> {
  return service.put(url, data) as unknown as Promise<T>
}

export function del<T = unknown>(url: string): Promise<T> {
  return service.delete(url) as unknown as Promise<T>
}

export function upload<T = unknown>(url: string, formData: FormData): Promise<T> {
  return service.post(url, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  }) as unknown as Promise<T>
}
