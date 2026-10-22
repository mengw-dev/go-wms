/** Keyboard scanners send Enter separately. Keep zeros and code case intact. */
export function scanCode(value: string): string {
  const code = value.trim()
  if (!code || [...code].length > 64 || [...code].some(char => char.charCodeAt(0) < 32 || (char.charCodeAt(0) >= 127 && char.charCodeAt(0) <= 159))) throw Error('请输入1–64字符的完整编码，不含控制字符')
  return code
}
