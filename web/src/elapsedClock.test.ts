import { describe, expect, it } from 'vitest'
import { ElapsedClock } from './elapsedClock'

describe('local pending clock', () => {
  it('keeps ticking between delayed responses without moving backward on refresh', () => {
    const clock = new ElapsedClock()
    expect(clock.observe('request-a', 5000, 1000)).toBe(5000)
    expect(clock.tick(2000)).toBe(6000)
    expect(clock.tick(7000)).toBe(11000)
    expect(clock.observe('request-a', 9000, 7000)).toBe(11000)
    expect(clock.tick(8000)).toBe(12000)
    expect(clock.observe('request-b', 20000, 8000)).toBe(20000)
    clock.reset()
    expect(clock.tick(9000)).toBeNull()
  })
})
