// draw.io 代码块相关的纯函数：从 markdown 中提取 XML，以及更新文档里的"活动块"。

// 从（流式）助手消息正文中提取最后一个 ```drawio 块内的 XML。
// 容忍未闭合 fence（AI 仍在输出中）：找到 ```drawio 起始后，
// 若没有对应的 ``` 闭合，就取起始之后到文本末尾的全部内容。
export function extractLastDrawioXml(text: string): string {
  if (!text) return ''
  // 找到最后一个 ```drawio 起始位置（lastIndexOf 比 exec 循环更省）
  const startMarker = '```drawio'
  const altMarker = '```draw'
  let startIdx = text.lastIndexOf(startMarker)
  if (startIdx < 0) startIdx = text.lastIndexOf(altMarker)
  if (startIdx < 0) return ''
  // 跳过 marker 和同行换行
  const afterMarker = text.indexOf('\n', startIdx)
  if (afterMarker < 0) {
    // marker 后还没有换行（刚写完 fence 语言标记），视为尚未开始输出 XML
    return ''
  }
  const contentStart = afterMarker + 1
  // 找闭合 fence：必须独占一行（``` 或 ```xxx）
  const tail = text.slice(contentStart)
  const closeRe = /\n```[^\n]*$|\n```(?=\n|$)/
  const closeMatch = tail.match(closeRe)
  if (closeMatch && closeMatch.index !== undefined) {
    return tail.slice(0, closeMatch.index).replace(/\n$/, '')
  }
  // 未闭合（流式中）→ 取到末尾
  return tail.replace(/\n$/, '')
}

// 把新的 drawio XML 写入文档里"最后一个" ```drawio 块（活动块机制）。
// 若文档中尚无 drawio 块，则在末尾追加一个。
// 返回更新后的文档内容（纯函数，不改入参）。
export function updateActiveDrawioBlock(cur: string, xml: string): string {
  const body = xml.replace(/\n$/, '')
  // 找最后一个 ```drawio 起始（容忍 ```draw 别名）
  const openIdx1 = cur.lastIndexOf('```drawio')
  const openIdx2 = cur.lastIndexOf('```draw')
  const openIdx = Math.max(openIdx1, openIdx2)
  if (openIdx < 0) {
    // 没有现成块 → 末尾追加
    const block = '```drawio\n' + body + '\n```\n'
    const sep = cur && !cur.endsWith('\n') ? '\n\n' : (cur.endsWith('\n\n') || !cur) ? '' : '\n'
    return cur + sep + block
  }
  // 从 open 之后的行起找闭合 fence：行首（可选空白）+ ```
  const afterOpen = cur.indexOf('\n', openIdx)
  if (afterOpen < 0) {
    // 起始 fence 后还没有换行 → 起始 fence 之后全部替换为 body + 闭合
    return cur.slice(0, openIdx) + '```drawio\n' + body + '\n```'
  }
  const linesStart = afterOpen + 1
  const rest = cur.slice(linesStart)
  const closeRe = /(^|\n)([^\S\n]*)```/
  const cm = rest.match(closeRe)
  if (cm && cm.index !== undefined) {
    const innerEnd = cm.index + cm[1].length // 闭合 fence 前的换行位置（相对 rest）
    const head = cur.slice(0, openIdx)
    const keptOpen = cur.slice(openIdx, linesStart) // 保留原 fence 行（含语言标记）
    const tail = rest.slice(innerEnd) // 从闭合 fence 开始到末尾（保留闭合 fence 及其后内容）
    return head + keptOpen + body + '\n' + tail
  }
  // 没有闭合 fence（异常）→ 直接重建该块到末尾
  return cur.slice(0, openIdx) + '```drawio\n' + body + '\n```'
}
