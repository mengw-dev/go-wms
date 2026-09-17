import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'
import { useAuthStore } from '@/stores/auth'

export class ApiError extends Error {
  code?: number
  status?: number

  constructor(message: string, code?: number, status?: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }
}

const service = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 20000,
})

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
        return Promise.reject(new ApiError(msg, body.code))
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
    } else if (status === 423 && code === 70003) {
      // 会话失效由业务流程中心统一处理和提示，避免自动刷新并发请求刷屏。
      useAuthStore().clearDemoSession()
    } else if (status === 423 && code === 70002) {
      ElMessage.warning(error?.response?.data?.msg || '当前环境正在被使用，请稍后重试')
    } else {
      const msg = error?.response?.data?.msg || error?.message || '网络异常'
      ElMessage.error(msg)
    }
    return Promise.reject(
      new ApiError(
        error?.response?.data?.msg || error?.message || '网络异常',
        error?.response?.data?.code,
        status,
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

export function post<T = unknown>(url: string, data?: unknown): Promise<T> {
  return service.post(url, data) as unknown as Promise<T>
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
