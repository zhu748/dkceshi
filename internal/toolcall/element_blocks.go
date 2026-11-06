package toolcall

// Pair each opening once, then select nonoverlapping complete blocks in source
// order. A broken outer block cannot trigger a new suffix scan for every child.
func collectElementBlocks(text string, next func(int) (ToolMarkupTag, bool)) []xmlElementBlock {
	var blocks []xmlElementBlock
	var bodyStarts, stack []int
	for pos := 0; pos < len(text); {
		tag, ok := next(pos)
		if !ok {
			break
		}
		pos = tag.End + 1
		if tag.Closing {
			if len(stack) > 0 {
				index := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				blocks[index].Body = text[bodyStarts[index]:tag.Start]
				blocks[index].End = pos
			}
			continue
		}
		if tag.SelfClosing {
			continue // Not an executable invocation/parameter shell.
		}
		attrsEnd := tag.End
		if size := xmlTagEndDelimiterLenEndingAt(text, tag.End); size > 0 {
			attrsEnd = tag.End + 1 - size
		}
		attrsEnd = max(attrsEnd, tag.NameEnd)
		stack = append(stack, len(blocks))
		blocks = append(blocks, xmlElementBlock{Start: tag.Start, Attrs: text[tag.NameEnd:attrsEnd]})
		bodyStarts = append(bodyStarts, pos)
	}
	var out []xmlElementBlock
	end := 0
	for _, block := range blocks {
		if block.End > 0 && block.Start >= end {
			out = append(out, block)
			end = block.End
		}
	}
	return out
}
