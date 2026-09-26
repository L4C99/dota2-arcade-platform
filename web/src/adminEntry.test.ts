import { describe, expect, it } from 'vitest'
import { instanceA2SLabel } from './adminEntry'

describe('administrator instance A2S diagnostic label', () => {
  it('distinguishes a missing fact from a failed query', () => {
    expect(instanceA2SLabel()).toBe('尚无查询事实')
    expect(instanceA2SLabel('ok')).toBe('查询正常')
    expect(instanceA2SLabel('failed')).toBe('查询失败')
  })
})
