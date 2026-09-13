import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'
import { useAuthStore } from '@/stores/auth'

const service = axios.create({
  baseURL: '/api/v1',
  timeout: 20000,
})

// 请求拦截器：注入 token
service.interceptors.request.use((config) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
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
        return Promise.reject(new Error(msg))
      }
      return body.data
    }
    return body
  },
  (error) => {
    const status = error?.response?.status
    if (status === 401) {
      useAuthStore().clear()
      if (router.currentRoute.value.path !== '/login') {
        ElMessage.error('登录已失效，请重新登录')
        router.push('/login')
      }
    } else {
      const msg = error?.response?.data?.msg || error?.message || '网络异常'
      ElMessage.error(msg)
    }
    return Promise.reject(error)
  },
)

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
