export function a2sStatus(enabled: boolean, queryOk: boolean): string {
  if (!enabled) return '未启用'
  return queryOk ? '当前 Ready 实例查询成功' : '当前没有成功的实时 A2S 查询结果'
}
