import { describe, expect, it } from 'vitest'
import { a2sStatus } from './adminEntry'

describe('administrator A2S diagnostic label', () => {
  it('does not call an idle node a query failure', () => {
    expect(a2sStatus(true, false)).toBe('当前没有成功的实时 A2S 查询结果')
    expect(a2sStatus(true, true)).toBe('当前 Ready 实例查询成功')
    expect(a2sStatus(false, false)).toBe('未启用')
  })
})
