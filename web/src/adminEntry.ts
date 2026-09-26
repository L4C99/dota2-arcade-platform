export function instanceA2SLabel(status?: 'ok' | 'failed'): string {
  if (status === 'ok') return '查询正常'
  if (status === 'failed') return '查询失败'
  return '尚无查询事实'
}
