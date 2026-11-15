// 删除下载歌曲时的确认文案与结果提示。
// 删除会同时删掉 NAS 上的音频文件，属于不可撤销操作，Web 端两个页面
// （「下载任务」与自己下载的任务、「下载管理」管理员全量列表）共用同一套文案。

const songLabel = (item) => {
  const name = item?.songName || ''
  const artist = item?.artist || ''
  return artist ? `《${name} - ${artist}》` : `《${name}》`
}

// confirmDeleteText 返回二次确认弹窗的正文。
// 只有 done 状态的记录才保证 NAS 上存在音频文件，未完成的任务只清理记录与临时文件。
export const confirmDeleteText = (item) => {
  const label = songLabel(item)
  if (item?.status && item.status !== 'done') {
    return [
      `确定删除${label}这条下载记录？`,
      '',
      '该任务尚未完成入库，删除只会移除任务记录（如已产生临时文件也会一并清理）。',
      '之后可以重新搜索下载。',
    ].join('\n')
  }
  return [
    `确定删除${label}？`,
    '',
    '会同时删除：',
    '· NAS 上的音频文件',
    '· 曲库中的歌曲记录',
    '· 下载记录',
    '',
    '删除后该歌曲不在曲库，可以重新下载。此操作不可撤销。',
  ].join('\n')
}

// deleteResultText 把服务端返回的 deletedFile/deletedDb/warnings 转成一句结果提示，
// 让用户明确知道 NAS 上的文件到底有没有删掉。
export const deleteResultText = (resp, item) => {
  const label = songLabel(item)
  const deletedFile = !!resp?.deletedFile
  const deletedDb = Number(resp?.deletedDb || 0)
  const warnings = Array.isArray(resp?.warnings) ? resp.warnings : []
  const warnText = warnings.length ? `（${warnings.join('；')}）` : ''

  if (!deletedFile && deletedDb === 0) {
    return `${label}的记录已删除，但 NAS 上的音频文件未删除${warnText}`
  }

  const parts = [deletedFile ? `${label}已删除，音频文件已从 NAS 移除` : `${label}已删除，NAS 上无对应音频文件`]
  if (deletedDb > 0) parts.push(`曲库记录 ${deletedDb} 条`)
  return parts.join(' · ') + warnText
}