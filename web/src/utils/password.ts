import type { FormItemRule } from 'element-plus'

/** bcrypt 以 UTF-8 字节计长，不能只检查字符数。 */
export const passwordByteRule: FormItemRule = {
  trigger: 'blur',
  validator: (_rule, value: string, done) => {
    done(new TextEncoder().encode(value ?? '').length > 72 ? new Error('密码不能超过 72 字节，请减少多字节字符') : undefined)
  },
}
