# API -> 缃戦〉瀵硅瘽绾枃鏈吋瀹逛富閾捐矾璇存槑

鏂囨。瀵艰埅锛歔鎬昏](../README.MD) / [鏋舵瀯璇存槑](./ARCHITECTURE.md) / [鎺ュ彛鏂囨。](../API.md) / [娴嬭瘯鎸囧崡](./TESTING.md)

> 鏈枃妗ｆ槸 DS2API鈥滄妸 OpenAI / Claude / Gemini 椋庢牸 API 璇锋眰鍏煎鎴?DeepSeek 缃戦〉瀵硅瘽绾枃鏈笂涓嬫枃鈥濈殑涓撻」璇存槑銆?> 杩欐槸椤圭洰鏈€閲嶈鐨勫吋瀹逛骇鐗╀箣涓€銆傚嚒鏄慨鏀规秷鎭爣鍑嗗寲銆乼ool prompt 娉ㄥ叆銆乼ool history 淇濈暀銆佹枃浠跺紩鐢ㄣ€乧urrent input file銆佷笅娓?completion payload 缁勮绛夎涓猴紝閮藉繀椤诲悓姝ユ洿鏂版湰鏂囨。銆?
## 1. 鏍稿績缁撹

DS2API 褰撳墠鐨勬牳蹇冩€濊矾锛屼笉鏄妸瀹㈡埛绔紶鏉ョ殑 `messages`銆乣tools`銆乣attachments` 鍘熸牱杞彂缁欎笅娓搞€?
鑰屾槸鎶婅繖浜涢珮灞?API 璇箟锛岀粺涓€鍘嬬缉鎴?DeepSeek 缃戦〉瀵硅瘽鏇村鏄撶悊瑙ｇ殑涓夌被杈撳叆锛?
1. `prompt`
   涓€涓崟瀛楃涓诧紝閲岄潰甯︽湁瑙掕壊鏍囪銆乻ystem 鎸囦护銆佸巻鍙叉秷鎭€乤ssistant reasoning 鏍囩銆佸巻鍙?tool call XML 绛夈€?2. `ref_file_ids`
   涓€涓枃浠跺紩鐢ㄦ暟缁勶紝鎵胯浇闄勪欢銆乮nline 涓婁紶鏂囦欢锛屼互鍙婂繀瑕佹椂琚媶鍑哄幓鐨勫巻鍙叉枃浠躲€?3. 鎺у埗浣?   渚嬪 `thinking_enabled`銆乣search_enabled`銆侀儴鍒?passthrough 鍙傛暟銆?
涔熷氨鏄锛岄」鐩渶閲嶈鐨勫吋瀹瑰姩浣滐紝鏄妸鈥滅粨鏋勫寲 API 浼氳瘽鈥濈炕璇戞垚鈥滅綉椤靛璇濈函鏂囨湰涓婁笅鏂?+ 鏂囦欢寮曠敤鈥濄€?
## 2. 涓轰粈涔堣繖鏄牳蹇冧骇鐗?
鍥犱负瀵逛笅娓告潵璇达紝鐪熸绋冲畾鐨勮緭鍏ラ潰涓嶆槸 OpenAI/Claude/Gemini 鐨勫師鐢?schema锛岃€屾槸锛?
- 涓€娈佃繛缁殑瀵硅瘽 prompt
- 涓€缁勫彲寮曠敤鏂囦欢
- 灏戦噺寮€鍏充綅

杩欎篃鏄负浠€涔堝緢澶氳〃闈笂鐪嬪儚鈥滃崗璁吋瀹光€濈殑浠ｇ爜锛屾渶缁堥兘浼氭敹鏁涘埌鍚屼竴绫婚€昏緫锛?
- 鍏堟妸涓嶅悓鍗忚鐨勬秷鎭粺涓€鎴愬唴閮ㄦ秷鎭簭鍒?- 鍐嶆妸宸ュ叿澹版槑鏀瑰啓鎴?system prompt 鏂囨湰
- 鍐嶆妸鍘嗗彶 tool call / tool result 鏀瑰啓鎴?prompt 鍙鍐呭
- 鏈€鍚庤緭鍑烘垚 DeepSeek completion payload

## 3. 缁熶竴蹇冩櫤妯″瀷

褰撳墠涓婚摼璺彲浠ヨ繖鏍风悊瑙ｏ細

```text
瀹㈡埛绔姹?  -> HTTP API surface锛圤penAI / Claude / Gemini锛?  -> promptcompat 缁熶竴娑堟伅鏍囧噯鍖?  -> tool prompt 娉ㄥ叆
  -> DeepSeek 椋庢牸 prompt 鎷艰
  -> 鏂囦欢鏀堕泦 / inline 涓婁紶锛圤penAI 鏂囦欢閾捐矾锛?  -> current input file锛坈ompletion runtime 鍏ㄥ眬鍏ュ彛锛?  -> completion payload
  -> 涓嬫父缃戦〉瀵硅瘽鎺ュ彛
  -> assistantturn 杈撳嚭璇箟褰掍竴锛圙o 闈炴祦寮?+ 娴佸紡鏀跺熬锛?  -> 鍚勫崗璁?renderer锛圤penAI / Responses / Claude / Gemini锛?```

瀵瑰簲鐨勫叧閿唬鐮佸叆鍙ｏ細

- OpenAI Chat / Responses锛?  [internal/promptcompat/request_normalize.go](../internal/promptcompat/request_normalize.go)
- OpenAI prompt 缁勮锛?  [internal/promptcompat/prompt_build.go](../internal/promptcompat/prompt_build.go)
- OpenAI 娑堟伅鏍囧噯鍖栵細
  [internal/promptcompat/message_normalize.go](../internal/promptcompat/message_normalize.go)
- Claude 鏍囧噯鍖栵細
  [internal/httpapi/claude/standard_request.go](../internal/httpapi/claude/standard_request.go)
- Claude 娑堟伅涓?tool_use/tool_result 褰掍竴锛?  [internal/httpapi/claude/handler_utils.go](../internal/httpapi/claude/handler_utils.go)
- Gemini 澶嶇敤 OpenAI prompt builder锛?  [internal/httpapi/gemini/convert_request.go](../internal/httpapi/gemini/convert_request.go)
- DeepSeek prompt 瑙掕壊鏍囪鎷艰锛?  [internal/prompt/messages.go](../internal/prompt/messages.go)
- prompt 鍙 tool history XML锛?  [internal/prompt/tool_calls.go](../internal/prompt/tool_calls.go)
- 鏈€鏂?user 鎬濊€冩牸寮忔敞鍏ワ細
  [internal/promptcompat/thinking_injection.go](../internal/promptcompat/thinking_injection.go)
- completion payload锛?  [internal/promptcompat/standard_request.go](../internal/promptcompat/standard_request.go)
- Go 杈撳嚭渚?assistant turn锛?  [internal/assistantturn/turn.go](../internal/assistantturn/turn.go)
- Go completion runtime锛?  [internal/completionruntime/nonstream.go](../internal/completionruntime/nonstream.go)

## 4. 涓嬫父鐪熸鏀跺埌鐨勪笢瑗?
鍦ㄢ€滃畬鎴愭爣鍑嗗寲鍚庘€濓紝涓嬫父 completion payload 鐨勬牳蹇冨舰鎬佹槸锛?
```json
{
  "chat_session_id": "session-id",
  "model_type": "default",
  "parent_message_id": null,
  "prompt": "<|begin鈻乷f鈻乻entence|>...",
  "ref_file_ids": [
    "file-history",
    "file-systemprompt",
    "file-other-attachment"
  ],
  "thinking_enabled": true,
  "search_enabled": false
}
```

閲嶇偣鏄細

- `prompt` 鎵嶆槸瀵硅瘽涓婁笅鏂囦富杞戒綋銆?- `ref_file_ids` 鍙壙杞芥枃浠跺紩鐢紝涓嶆壙杞芥櫘閫氭枃鏈秷鎭€?- `tools` 涓嶄細浣滀负鈥滃師鐢熷伐鍏?schema鈥濈洿鎺ヤ笅鍙戠粰涓嬫父锛岃€屾槸琚敼鍐欒繘 `prompt`銆?- 瀵瑰杩斿洖缁欏鎴风鐨?`prompt_tokens` / `input_tokens` / `promptTokenCount` 涓嶅啀鎸夆€滄渶鍚庝竴鏉℃秷鎭€濇垨瀛楃绮椾及杩戜技杩斿洖锛岃€屾槸鍩轰簬**瀹屾暣涓婁笅鏂?prompt**鍋?tokenizer 璁℃暟锛涗负浜嗛伩鍏嶄笂涓嬫枃瀹為檯瓒呴檺浣嗗鎴风璇互涓鸿繕鑳藉涓嬶紝璇锋眰渚т笂涓嬫枃 token 浼氶澶栦繚瀹堜笂娴竴鐐癸紝瀹佸彲鐣ュぇ涔熶笉浣庝及銆?- 褰撳墠 `/v1/chat/completions` 涓氬姟璺緞浠嶆槸鈥滄瘡娆¤姹傛柊寤轰竴涓繙绔?`chat_session_id`锛屽苟榛樿鍙戦€?`parent_message_id: null`鈥濓紱鍥犳 DS2API 瀵瑰榛樿琛ㄧ幇涓衡€滄柊浼氳瘽 + prompt 鎷煎巻鍙测€濓紝鑰屼笉鏄鐢?DeepSeek 鍘熺敓浼氳瘽鏍戙€?- 浣?DeepSeek 杩滅鏈韩鏀寔鍚屼竴 `chat_session_id` 鐨勮法杞鎸佺画瀵硅瘽銆?026-04-27 宸茬敤椤圭洰鍐呯幇鏈?DeepSeek client 鍋氳繃涓€娆′笉鏀逛笟鍔′唬鐮佺殑鍙岃疆瀹炴祴锛氬悓涓€ `chat_session_id` 涓嬶紝绗?1 杞繑鍥?`request_message_id=1` / `response_message_id=2` / 鏂囨湰 `SESSION_TEST_ONE`锛涚 2 杞噸鏂拌幏鍙栦竴娆?PoW锛屽苟鍙戦€?`parent_message_id=2` 鍚庯紝鎴愬姛杩斿洖 `request_message_id=3` / `response_message_id=4` / 鏂囨湰 `SESSION_TEST_TWO`銆傝繖璇存槑鈥滃悓杩滅浼氳瘽鎸佺画鑱婂ぉ鈥濊兘鍔涘瓨鍦紝涓旀瘡杞渶瑕佹惡甯︽纭殑 parent/message 閾炬帴淇℃伅锛屽悓鏃堕噸鏂拌幏鍙栧搴旇疆娆″彲鐢ㄧ殑 PoW銆?- OpenAI Chat / Responses 鍘熺敓璧扮粺涓€ OpenAI 鏍囧噯鍖栦笌 DeepSeek payload 缁勮锛汣laude / Gemini 浼氬敖閲忓鐢?OpenAI prompt/tool 璇箟锛屽叾涓?Gemini 鐩存帴澶嶇敤 `promptcompat.BuildOpenAIPromptForAdapter`銆侴o 涓绘湇鍔℃柊澧?`completionruntime` 鍚姩灞傦紝缁熶竴鎵ц DeepSeek session/PoW/call锛涜緭鍑轰晶鏂板 `assistantturn` 璇箟灞傦細闈炴祦寮?OpenAI Chat / Responses / Claude / Gemini 浼氭妸 DeepSeek SSE 鏀堕泦缁撴灉鍏堝綊涓€鎴愬悓涓€浠?assistant turn锛屽啀鍒嗗埆娓叉煋鎴愬悇鍗忚鍘熺敓澶栧舰锛涙祦寮?OpenAI Chat / Responses / Claude / Gemini 缁х画淇濇寔鍚勫崗璁疄鏃?SSE framing锛屼絾鏈€缁堟敹灏剧殑 tool fallback銆乻chema 褰掍竴銆乽sage銆乪mpty-output / content-filter 閿欒璇箟鍚屾牱鐢?`assistantturn` 鍒ゅ畾銆侰laude / Gemini 鐨勫父瑙?Go 涓昏矾寰勪笉鍐嶄緷璧栧唴閮?`httptest` 杞彂鍒?OpenAI handler锛沗translatorcliproxy` 浠呬繚鐣欑敤浜?Vercel bridge銆佸悗绔己澶?fallback 鍜屽洖褰掓祴璇曪紝涓嶄綔涓轰富涓氬姟鍗忚杞崲涓績銆?- Vercel Node 娴佸紡璺緞鏈疆涓嶈縼绉伙紝浠嶄娇鐢ㄧ幇鏈?Node bridge / stream-tool-sieve 瀹炵幇锛涘悗缁嫢鍙樻洿 Node 娴佸紡璇箟锛岄渶瑕佹寜 `assistantturn` 鐨?Go canonical 杈撳嚭璇箟鍚屾瀵归綈銆?- 瀹㈡埛绔紶鍏ョ殑 thinking / reasoning 寮€鍏充細琚綊涓€鍒颁笅娓?`thinking_enabled`銆侴emini `generationConfig.thinkingConfig.thinkingBudget` 浼氱炕璇戞垚鍚屼竴濂?thinking 寮€鍏筹紱鍏抽棴鏃跺嵆浣夸笂娓歌繑鍥?`response/thinking_content`锛屽吋瀹瑰眰涔熶笉浼氭妸瀹冨綋浣滃彲瑙佹鏂囪緭鍑恒€傝嫢鏈€缁堣В鏋愬嚭鐨勬ā鍨嬪悕甯?`-nothinking` 鍚庣紑锛屽垯浼氭棤鏉′欢寮哄埗鍏抽棴 thinking锛屼紭鍏堢骇楂樹簬璇锋眰浣撲腑鐨?`thinking` / `reasoning` / `reasoning_effort`銆傛湭鏄惧紡鍏抽棴鏃讹紝鍚?surface 浼氭寜瑙ｆ瀽鍚庣殑 DeepSeek 妯″瀷榛樿鑳藉姏寮€鍚?thinking锛屽苟鐢ㄥ悇鑷崗璁殑鍘熺敓褰㈡€佹毚闇诧細OpenAI Chat 涓?`reasoning_content`锛孫penAI Responses 涓?`response.reasoning.delta` / `reasoning` content锛孋laude 涓?`thinking` block / `thinking_delta`锛孏emini 涓?`thought: true` part銆?- 瀵?OpenAI Chat / Responses 鐨勯潪娴佸紡鏀跺熬锛屽鏋滄渶缁堝彲瑙佹鏂囦负绌猴紝鍏煎灞備細浼樺厛灏濊瘯鎶婃€濈淮閾句腑鐨勭嫭绔?DSML / XML 宸ュ叿鍧楀綋浣滅湡瀹炲伐鍏疯皟鐢ㄨВ鏋愬嚭鏉ャ€傛祦寮忛摼璺篃浼氬湪鏀跺熬闃舵鍋氬悓鏍风殑 fallback 妫€娴嬶紝浣嗕笉浼氬洜涓烘€濈淮閾惧唴瀹瑰幓涓€旀嫤鎴垨鏀瑰啓娴佸紡杈撳嚭锛涚湡姝ｇ殑宸ュ叿璇嗗埆濮嬬粓鍩轰簬鍘熷涓婃父鏂囨湰锛岃€屼笉鏄熀浜庘€滃凡缁忓仛杩囧彲瑙佽緭鍑烘竻娲椻€濈殑鐗堟湰銆傛渶缁堝彲瑙佸眰浼氬墺绂诲凡缁忔垚鍔熻В鏋愭垚宸ュ叿璋冪敤鐨勫畬鏁?leaked DSML / XML `tool_calls` wrapper锛涘鏋滈亣鍒板畬鏁?wrapper 浣嗗唴閮ㄥ舰鎬佷笉绗﹀悎鍙墽琛屽伐鍏疯皟鐢ㄨ涔夛紙渚嬪 `<param>` 杩欑被 malformed XML 宸ュ叿澹筹級锛屾祦寮?sieve 浼氭妸璇ュ潡浣滀负鏅€氭枃鏈噴鏀撅紝鑰屼笉鏄悶鎺夋垨浼€犳垚宸ュ叿璋冪敤銆傝ˉ鍙戠粨鏋滀細浣滀负鏈疆 assistant 鐨勭粨鏋勫寲 `tool_calls` / `function_call` 杈撳嚭杩斿洖锛岃€屼笉鏄杩?`content` 鏂囨湰锛涘鏋滃鎴风娌℃湁寮€鍚?thinking / reasoning锛屾€濈淮閾惧彧鐢ㄤ簬妫€娴嬶紝涓嶄細浣滀负 `reasoning_content` 鎴栧彲瑙佹鏂囨毚闇层€傚彧鏈夋鏂囦负绌轰笖鎬濈淮閾鹃噷涔熸病鏈夊彲鎵ц宸ュ叿璋冪敤鏃讹紝鎵嶇户缁寜绌哄洖澶嶉敊璇鐞嗐€?- OpenAI Chat / Responses銆丆laude Messages銆丟emini generateContent 鐨勭┖鍥炲閿欒澶勭悊涔嬪墠浼氶粯璁ゅ仛涓€娆″唴閮ㄨˉ鍋块噸璇曪細绗竴娆′笂娓稿畬鏁寸粨鏉熷悗锛屽鏋滄渶缁堝彲瑙佹鏂囦负绌恒€佹病鏈夎В鏋愬埌宸ュ叿璋冪敤銆佷篃娌℃湁宸茬粡鍚戝鎴风娴佸紡鍙戝嚭宸ュ叿璋冪敤锛屽苟涓旂粓姝㈠師鍥犱笉鏄?`content_filter`锛屽吋瀹瑰眰浼氬鐢ㄥ悓涓€涓?`chat_session_id`銆佽处鍙枫€乼oken 涓庡伐鍏风瓥鐣ワ紝鎶婂師濮?completion `prompt` 杩藉姞鍥哄畾鍚庣紑 `Previous reply had no visible output. Please regenerate the visible final answer or tool call now.` 鍚庨噸鏂版彁浜や竴娆°€侴o 涓昏矾寰勭殑闈炴祦寮忛噸璇曠敱 `completionruntime.ExecuteNonStreamWithRetry` 缁熶竴澶勭悊锛涙祦寮忛噸璇曠敱 `completionruntime.ExecuteStreamWithRetry` 缁熶竴澶勭悊锛屽悇鍗忚 runtime 鍙礋璐ｆ秷璐?娓叉煋鏈崗璁?SSE framing銆傞噸璇曢伒寰?DeepSeek 澶氳疆瀵硅瘽鍗忚锛氫粠绗竴娆′笂娓?SSE 娴佷腑鎻愬彇 `response_message_id`锛屽苟鍦ㄩ噸璇?payload 涓缃?`parent_message_id` 涓鸿鍊硷紝浣块噸璇曟垚涓哄悓涓€浼氳瘽鐨勫悗缁疆娆¤€岄潪鏂鐨勬牴娑堟伅锛涘悓鏃堕噸鏂拌幏鍙栦竴娆?PoW锛堣嫢 PoW 鑾峰彇澶辫触鍒欏洖閫€鍒板師濮?PoW锛夈€傝鍚岃处鍙烽噸璇曚笉浼氶噸鏂版爣鍑嗗寲娑堟伅銆佷笉浼氭柊寤?session锛屼篃涓嶄細鍚戞祦寮忓鎴风鎻掑叆閲嶈瘯鏍囪锛涚浜屾 thinking / reasoning 浼氭寜姝ｅ父澧為噺鐩存帴鎺ュ埌绗竴娆′箣鍚庯紝骞剁户缁娇鐢?overlap trim 鍘婚噸銆傝嫢鍚岃处鍙疯ˉ鍋块噸璇曞悗鍗冲皢杩斿洖 429 `upstream_empty_output`锛屽苟涓斿綋鍓嶆槸鎵樼璐﹀彿妯″紡锛宺untime 浼氬湪杩斿洖 429 鍓嶅垏鎹㈠埌涓嬩竴涓彲鐢ㄨ处鍙凤紝鏂板缓 `chat_session_id`锛屼娇鐢ㄥ師濮?completion payload 鍐嶅仛涓€娆?fresh retry锛涜鍒囧彿閲嶈瘯涓嶆惡甯︾┖鍥炲 prompt 鍚庣紑锛屼篃涓嶈缃笂涓€璐﹀彿鐨?`parent_message_id`銆傚鏋?current input file 宸茶Е鍙戯紝鍒囧彿鍓嶄細鍦ㄦ柊璐﹀彿涓婇噸鏂颁笂浼犲悓涓€浠?`chat_context.txt`锛堜互鍙婇渶瑕佹椂鐨?`tool_schema.txt`锛夛紝骞剁敤鏂拌处鍙峰彲瑙佺殑 file_id 鏇挎崲鑷姩鐢熸垚鐨勬棫 file_id锛涘鎴风鍘熸湰浼犲叆鐨勫叾浠栨枃浠跺紩鐢ㄤ繚鎸佷笉鍙樸€傚鏋滄病鏈夊彲鍒囨崲璐﹀彿锛屾垨鍒囧彿鍚庣殑 fresh retry 浠嶆病鏈夊彲瑙佹鏂囨垨宸ュ叿璋冪敤锛屽垯缁х画鎸夊師閿欒杩斿洖锛氭棤浠讳綍杈撳嚭涓?503 `upstream_unavailable`锛屾湁 reasoning 浣嗘病鏈夊彲瑙佹鏂囨垨宸ュ叿璋冪敤涓?429 `upstream_empty_output`銆傝嫢浠讳竴灏濊瘯瑙﹀彂绌?`content_filter`锛屼笉鍋氳ˉ鍋块噸璇曞苟淇濇寔 `content_filter` 閿欒銆俈ercel Node 娴佸紡璺緞閫氳繃 Go 鍐呴儴 prepare / pow / switch 绔偣鑾峰彇鍒濆 payload銆侀噸璇?PoW 鍜屽垏鍙?fresh retry payload锛屽洜姝ゅ悓鏍蜂細閲嶆柊涓婁紶 current-input 鑷姩鏂囦欢骞舵浛鎹负鏂拌处鍙?file_id銆?
- 闈炴祦寮?OpenAI Chat / Responses銆丆laude Messages銆丟emini generateContent 鍦ㄦ渶缁堝彲瑙佹鏂囨覆鏌撻樁娈碉紝浼氭妸 DeepSeek 鎼滅储杩斿洖涓殑 `[citation:N]` / `[reference:N]` 鏍囪鏇挎崲鎴愬搴?Markdown 閾炬帴銆俙citation` 鏍囪鎸変竴鍩哄簭鍙疯В鏋愶紱`reference` 鏍囪鍙湁鍦ㄥ悓涓€娈垫鏂囦腑鍑虹幇 `[reference:0]`锛堝厑璁稿啋鍙峰悗鏈夌┖鏍硷級鏃舵墠鎸夐浂鍩哄簭鍙锋槧灏勶紝骞朵笖涓嶄細褰卞搷鍚屾姝ｆ枃閲岀殑 `citation` 鏍囪銆?- 娴佸紡杈撳嚭浠嶉粯璁ら殣钘?`[citation:N]` / `[reference:N]` 杩欑被涓婃父鍐呴儴鏍囪锛岄伩鍏嶅垎鐗囪緭鍑轰腑娉勬紡灏氭湭瀹屾垚鏄犲皠鐨勫紩鐢ㄥ崰浣嶇銆?
## 5. prompt 鏄€庝箞鎷煎嚭鏉ョ殑

OpenAI Chat / Responses 鍦ㄦ爣鍑嗗寲鍚庛€乧urrent input file 涔嬪墠锛屼細榛樿鎵ц `thinking_injection` 澧炲己銆傚畠鍙傝€?DeepSeek V4 鈥滄妸鎺у埗鎸囦护鏀惧湪 user 娑堟伅鏈熬鏇寸ǔ瀹氣€濈殑鐢ㄦ硶锛屽湪鏈€鏂?user message 鍚庤拷鍔犳€濊€冨寮烘彁绀鸿瘝銆傚綋鍓嶅唴缃粯璁ゆ彁绀鸿瘝浠?`Reasoning Effort: Absolute maximum with no shortcuts permitted.` 寮€澶达紝骞剁户缁姹傛ā鍨嬪厖鍒嗗垎瑙ｉ棶棰樸€佽鐩栨綔鍦ㄨ矾寰勪笌杈圭晫鏉′欢銆佹妸瀹屾暣鎺ㄦ紨杩囩▼鏄惧紡鍐欏嚭銆傝寮€鍏抽粯璁ゅ惎鐢紝鍙€氳繃 `thinking_injection.enabled=false` 鍏抽棴锛涗篃鍙互閫氳繃 `thinking_injection.prompt` 鑷畾涔夋彁绀鸿瘝锛岀暀绌烘椂浣跨敤鍐呯疆榛樿鎻愮ず璇嶃€?
杩欐澧炲己灞炰簬 prompt 鍙涓婁笅鏂囷細

- 鏅€氳姹備細鐩存帴鍑虹幇鍦ㄦ渶缁?`prompt` 鐨勬渶鏂?user block 鏈熬銆?- 濡傛灉瑙﹀彂 current input file锛屽畠浼氳繘鍏ュ畬鏁翠笂涓嬫枃鏂囦欢涓€?
鍙﹀锛宍MessagesPrepareWithThinking` 杩樹細鍦ㄦ渶缁?prompt 鐨勬渶鍓嶉潰棰勭疆涓€娈靛浐瀹氱殑 system 绾р€滆緭鍑哄畬鏁存€х害鏉燂紙Output integrity guard锛夆€濓細

- 濡傛灉涓婃父涓婁笅鏂囥€佸伐鍏疯緭鍑烘垨瑙ｆ瀽鍚庣殑鏂囨湰鍑虹幇涔辩爜銆佹崯鍧忋€侀儴鍒嗚В鏋愩€侀噸澶嶆垨鍏朵粬鐣稿舰鐗囨锛屼笉瑕佹ā浠裤€佷笉瑕佸洖鏄撅紝鍙緭鍑虹粰鐢ㄦ埛鐨勬纭唴瀹广€?- 杩欐绾︽潫浣嶄簬鏅€?system / tool prompt 涔嬪墠锛屽洜姝ゆ槸褰撳墠鏈€缁?prompt 閲岀殑鏈€楂樹紭鍏堢骇鍓嶇疆鎸囦护銆?
### 5.1 瑙掕壊鏍囪

鏈€缁?prompt 浣跨敤 DeepSeek 椋庢牸瑙掕壊鏍囪锛?
- `<|begin鈻乷f鈻乻entence|>`
- `<|System|>`
- `<|User|>`
- `<|Assistant|>`
- `<|Tool|>`
- `<|end鈻乷f鈻乮nstructions|>`
- `<|end鈻乷f鈻乻entence|>`
- `<|end鈻乷f鈻乼oolresults|>`

瀹炵幇浣嶇疆锛?[internal/prompt/messages.go](../internal/prompt/messages.go)

### 5.2 鐩搁偦鍚岃鑹叉秷鎭細鍚堝苟

鍦ㄦ渶缁?`MessagesPrepareWithThinking` 涓紝鐩搁偦鍚?role 鐨勬秷鎭細琚悎骞舵垚涓€涓潡锛屼腑闂存彃鍏ョ┖琛屻€?
杩欐剰鍛崇潃锛?
- prompt 涓湅鍒扮殑鏄€滃悎骞跺悗鐨?role block鈥?- 涓嶆槸瀹㈡埛绔紶鏉ョ殑閫愭潯 message 鍘熸牱鎺掑垪

## 6. tools 涓轰粈涔堟槸鈥滄枃鏈敞鍏モ€濓紝涓嶆槸鍘熺敓涓嬪彂

褰撳墠椤圭洰鎶婂伐鍏疯兘鍔涜涓衡€減rompt 绾︽潫鐨勪竴閮ㄥ垎鈥濄€?
鍏蜂綋鍋氭硶锛?
1. 鎶婃瘡涓?tool 鐨勫悕绉般€佹弿杩般€佸弬鏁?schema 搴忓垪鍖栨垚鏂囨湰銆?2. 鎷兼垚 `You have access to these tools:` 澶ф璇存槑銆?3. 鍐嶉檮涓婄粺涓€鐨?DSML tool call 澶栧３鏍煎紡绾︽潫銆?4. 鏅€氱洿浼犺姹備細鎶娾€滃伐鍏锋弿杩?+ 鏍煎紡绾︽潫鈥濅竴璧峰苟鍏?system prompt锛涘鏋?`current_input_file` 瑙﹀彂锛屽垯宸ュ叿鎻忚堪/schema 浼氬崟鐙笂浼犳垚 `tool_schema.txt`锛宭ive prompt 鍜?system tool 鏍煎紡鎻愮ず閮戒細鏄庣‘瑕佹眰妯″瀷鎶?`tool_schema.txt` 褰撲綔鍙皟鐢ㄥ伐鍏峰拰鍙傛暟 schema 鐨勬潈濞佹潵婧愩€?
宸ュ叿璋冪敤姝ｄ緥鐜板湪浼樺厛绀鸿寖鍗婅绠￠亾绗?DSML 椋庢牸锛歚<|DSML|tool_calls>` 鈫?`<|DSML|invoke name="...">` 鈫?`<|DSML|parameter name="...">`銆?鍏煎灞備粛鎺ュ彈鏃у紡绾?`<tool_calls>` wrapper锛屽苟浼氬閿欒嫢骞?DSML 鏍囩鍙樹綋锛屽寘鎷煭妯嚎褰㈠紡 `<dsml-tool-calls>` / `<dsml-invoke>` / `<dsml-parameter>`銆佷笅鍒掔嚎褰㈠紡 `<dsml_tool_calls>` / `<dsml_invoke>` / `<dsml_parameter>`锛屼互鍙婂叾浠栧墠缂€鍒嗛殧褰㈡€佸 `<vendor|tool_calls>` / `<vendor_tool_calls>` / `<vendor - tool_calls>`锛涙爣绛惧３鎵弿杩樹細鎶婂叏瑙?ASCII 婕傜Щ褰掍竴鍖栵紝渚嬪 `<锝勶汲锛棘|tool_calls>` 涓庡叏瑙?`锛瀈 缁撴潫绗︼紝涔熶細瀹归敊 CJK 灏栨嫭鍙枫€佸叏瑙掓劅鍙瑰彿鎴栭】鍙峰垎闅旂銆佸集寮曞彿灞炴€у€笺€丳ascalCase 鏈湴鍚嶅拰灞炴€у熬閮ㄥ垎闅旂婕傜Щ锛屼緥濡?`<DSM|parameter name="command"|>...銆?DSM|parameter銆塦銆乣<锛丏SML锛乮nvoke name=鈥淏ash鈥?`銆乣<銆丏SML銆乼ool_calls>`銆乣<DSmartToolCalls>`銆乣<DSMLtool_calls鈥?`銆傛洿涓€鑸湴锛孏o / Node tag 鎵弿浠ュ浐瀹氭湰鍦版爣绛惧悕 `tool_calls` / `invoke` / `parameter` 涓哄噯锛屾爣绛惧悕鍓嶆垨鏍囩鍚嶅悗鐨勯潪缁撴瀯鎬у崗璁垎闅旂閮戒細鍦ㄨВ鏋愬叆鍙ｅ墺绂伙紝渚嬪 `<DSML鈵倀ool_calls>`銆乣<proto馃挜tool_calls>` 杩欑被鎺у埗绗︽垨闈?ASCII 鍒嗛殧绗︽紓绉讳篃浼氬綊涓€鍖栧洖鐜版湁 XML 鏍囩鍚庣户缁蛋鍚屼竴濂?parser锛涚粨鏋勬€у瓧绗﹀ `<` / `>` / `/` / `=` / 寮曞彿銆佺┖鐧藉拰 ASCII 瀛楁瘝鏁板瓧涓嶄細琚綋浣滆繖绫诲垎闅旂銆傝繘鍏ョ幇鏈?DSML rewrite / XML parse 涔嬪墠锛孏o / Node 杩樹細鍏堝鈥滃凡缁忚瘑鍒垚宸ュ叿鏍囩澹崇殑 candidate span鈥濆仛涓€娆＄獎 canonicalization锛氬彧鎶樺彔 wrapper / `invoke` / `parameter` / `name` / `CDATA` / `DSML` 鍙婂叾澹冲眰鍒嗛殧绗﹂噷鐨?confusable 瀛楃锛屾竻鐞嗛浂瀹?/ BOM / 鎺у埗绫诲共鎵帮紝骞舵妸寮曞彿銆佺┖鐧姐€乨ash / underscore 鍙樹綋绛夌粺涓€鍥炲彲瑙ｆ瀽鐨勫伐鍏疯娉曘€傝繖涓樁娈典笉浼氬箍涔夋敼鍐欐櫘閫氭鏂囥€佸弬鏁板唴瀹广€丮arkdown 琛屽唴 code span銆丆DATA 閲岀殑绀轰緥鏂囨湰鎴栧叾浠栭潪宸ュ叿 XML銆侰DATA 寮€澶翠篃浣跨敤鍚屼竴绫绘壂鎻忓紡瀹归敊锛宍<![CDATA[` / `<锛乕CDATA[` / `<銆乕CDATA[` 閮戒細浣滀负鍙傛暟鍘熸枃瀹瑰櫒澶勭悊銆備絾鎻愮ず璇嶄細浼樺厛瑕佹眰妯″瀷杈撳嚭瀹樻柟 DSML 鏍囩锛屽苟寮鸿皟涓嶈兘鍙緭鍑?closing wrapper 鑰屾紡鎺?opening tag銆傞渶瑕佹敞鎰忥細杩欐槸鈥滃吋瀹?DSML 澶栧３锛屽唴閮ㄤ粛浠?XML 瑙ｆ瀽璇箟涓哄噯鈥濓紝涓嶆槸鍘熺敓 DSML 鍏ㄩ摼璺疄鐜般€傝В鏋愬櫒浼氬厛鎴幏闈?Markdown 浠ｇ爜涓婁笅鏂囦腑鐨勭枒浼煎伐鍏?wrapper锛屽畬鏁磋В鏋愬け璐ユ垨宸ュ叿璇箟鏃犳晥鏃跺啀鎸夋櫘閫氭枃鏈斁琛屻€?鏁扮粍鍙傛暟浣跨敤 `<item>...</item>` 瀛愯妭鐐硅〃绀猴紱褰撴煇涓弬鏁颁綋鍙寘鍚?item 瀛愯妭鐐规椂锛孏o / Node 瑙ｆ瀽鍣ㄤ細鎶婂畠杩樺師鎴愭暟缁勶紝閬垮厤 `questions` / `options` 杩欑被 schema 涓姹?array 鐨勫弬鏁拌璇В鏋愭垚 `{ "item": ... }` 瀵硅薄銆傞櫎姝や箣澶栵紝瑙ｆ瀽鍣ㄨ繕浼氬洖鏀朵竴浜涙洿鏉炬暎鐨勫垪琛ㄥ啓娉曪紝渚嬪 JSON array 瀛楅潰閲忔垨閫楀彿鍒嗛殧鐨?JSON 椤瑰簭鍒楋紝鍙瀹冧滑瓒冲鏄庣‘锛涗絾 `<item>` 浠嶇劧鏄閫夊舰鎬併€傝嫢妯″瀷鎶婂畬鏁寸粨鏋勫寲 XML fragment 璇寘杩?CDATA锛屽吋瀹瑰眰浼氬湪淇濇姢 `content` / `command` 绛夊師鏂囧瓧娈电殑鍓嶆彁涓嬶紝灏濊瘯鎶婇潪鍘熸枃瀛楁涓殑 CDATA XML fragment 杩樺師鎴?object / array銆備笉杩囷紝濡傛灉 CDATA 鍙槸鍗曚釜骞抽潰鐨?XML/HTML 鏍囩锛屼緥濡?`<b>urgent</b>` 杩欑琛屽唴鏍囪锛屽吋瀹瑰眰浼氫繚鐣欏師濮嬪瓧绗︿覆锛屼笉浼氬己琛屽崌鎴?object / array锛涘彧鏈夋槑鏄捐〃绀虹粨鏋勭殑 CDATA 鐗囨锛屼緥濡傚鍏勫紵鑺傜偣銆佸祵濂楀瓙鑺傜偣鎴?`item` 鍒楄〃锛屾墠浼氳Е鍙戠粨鏋勫寲鎭㈠銆傚 `command` / `content` 绛夐暱鏂囨湰鍙傛暟锛孋DATA 鍐呴儴鐨?Markdown fenced DSML / XML 绀轰緥浼氫綔涓哄師鏂囦繚鎶わ紱绀轰緥閲岀殑 `]]></parameter>` 鎴?`</tool_calls>` 涓嶄細鎴柇澶栧眰宸ュ叿璋冪敤锛岃В鏋愬櫒浼氱户缁瓑寰呭洿鏍忓鐪熸鐨勫弬鏁?/ wrapper 缁撴潫鏍囩銆?Go 渚ц鍙?DeepSeek SSE 鏃朵笉鍐嶄緷璧?`bufio.Scanner` 鐨勫浐瀹?2MiB 鍗曡涓婇檺锛涘綋鍐欐枃浠剁被宸ュ叿鎶婂緢闀跨殑 `content` 鏀惧湪鍗曚釜 `data:` 琛岄噷杩斿洖鏃讹紝闈炴祦寮忔敹闆嗐€佹祦寮忚В鏋愬拰 auto-continue 閫忎紶閮戒細淇濈暀瀹屾暣琛岋紝鍐嶈繘鍏ュ悓涓€濂楀伐鍏疯В鏋愪笌搴忓垪鍖栨祦绋嬨€?鍦?assistant 鏈€缁堝洖鍖呴樁娈碉紝濡傛灉鏌愪釜 tool 鍙傛暟鍦ㄥ０鏄?schema 涓槑纭槸 `string`锛屽吋瀹瑰眰浼氬湪鎶婅В鏋愬悗鐨?`tool_calls` / `function_call` 閲嶆柊搴忓垪鍖栨垚 OpenAI / Responses / Claude 鍙鍙傛暟鍓嶏紝閫掑綊鎶婅璺緞涓婄殑 number / bool / object / array 缁熶竴杞垚瀛楃涓诧紱鍏朵腑 object / array 浼氬帇鎴愮揣鍑?JSON 瀛楃涓层€傝繖涓繚鎶ゅ彧瀵?schema 鏄庣‘澹版槑涓?string 鐨勮矾寰勭敓鏁堬紝涓嶄細鏀瑰啓鏈潵灏辨槸 `number` / `boolean` / `object` / `array` 鐨勫弬鏁般€傝繖鏍峰彲浠ュ吋瀹?DeepSeek 杈撳嚭浜嗙粨鏋勫寲鐗囨銆佷絾涓婃父瀹㈡埛绔伐鍏?schema 鍙堜弗鏍艰姹傚瓧绗︿覆鍙傛暟鐨勫満鏅紙渚嬪 `content`銆乣prompt`銆乣path`銆乣taskId` 绛夛級銆?宸ュ叿 schema 鐨勬潈濞佹潵婧愬缁堟槸**褰撳墠璇锋眰瀹為檯鎼哄甫鐨?schema**锛岃€屼笉鏄悓鍚嶅伐鍏峰湪鍏朵粬 runtime锛圕laude Code / OpenCode / Codex 绛夛級閲岀殑榛樿鍗拌薄銆傚吋瀹瑰眰鐜板湪浼氬悓鏃跺吋瀹?OpenAI 椋庢牸 `function.parameters`銆佺洿鎺ュ伐鍏峰璞′笂鐨?`parameters` / `input_schema`銆佷互鍙?camelCase 鐨?`inputSchema` / `schema`锛屽苟鍦ㄦ渶缁堣緭鍑洪樁娈垫寜杩欎唤璇锋眰鍐?schema 鍐冲畾鏄繚鐣?array/object锛岃繕鏄粎瀵规槑纭０鏄庝负 `string` 鐨勮矾寰勫仛瀛楃涓插寲銆傝瑙勫垯鍚屾牱閫傜敤浜?Claude 鐨勬祦寮忔敹灏惧拰 Vercel Node 娴佸紡 tool-call formatter锛岄伩鍏嶄笉鍚?runtime 鍥?schema shape 宸紓鑰屽嚭鐜板悓鍚嶅伐鍏峰弬鏁扮被鍨嬫紓绉汇€?姝ｄ緥涓殑宸ュ叿鍚嶅彧浼氭潵鑷綋鍓嶈姹傚疄闄呭０鏄庣殑宸ュ叿锛涘鏋滃綋鍓嶈姹傛病鏈夎冻澶熺殑宸茬煡宸ュ叿褰㈡€侊紝灏辩渷鐣ュ搴旂殑鍗曞伐鍏枫€佸宸ュ叿鎴栧祵濂楃ず渚嬶紝閬垮厤鎶婁笉鍙敤宸ュ叿鍚嶅啓杩?prompt銆?瀵规墽琛岀被宸ュ叿锛岃剼鏈唴瀹瑰繀椤昏繘鍏ユ墽琛屽弬鏁版湰韬細`Bash` / `execute_command` 浣跨敤 `command`锛宍exec_command` 浣跨敤 `cmd`锛涗笉瑕佹妸鑴氭湰绀鸿寖鎴?`path` / `content` 鏂囦欢鍐欏叆鍙傛暟銆?宸ュ叿鎻愮ず璇嶄篃浼氭槑纭姹傛ā鍨嬫寜鏈璋冪敤瀹為檯闇€瑕佸～鍐欏弬鏁帮紝绂佹杈撳嚭 placeholder銆佺┖瀛楃涓叉垨绾┖鐧藉弬鏁帮紱濡傛灉蹇呭～鍙傛暟鏈煡锛屽簲鍏堣拷闂敤鎴锋垨姝ｅ父鏂囧瓧鍥炲锛岃€屼笉鏄緭鍑虹┖宸ュ叿澹炽€傚 `Bash` / `execute_command` 杩欑被 shell 宸ュ叿锛屽懡浠ゆ垨鑴氭湰蹇呴』鍐欏叆 `command` 鍙傛暟銆傝В鏋愬眰浠嶄細鎶婄┖瀛楃涓插弬鏁扮粨鏋勫寲杩斿洖锛涙槸鍚︽嫆缁濈┖ `command` 鐢卞悗缁伐鍏锋墽琛屼晶 / 瀹㈡埛绔?schema 鏍￠獙鍐冲畾銆?濡傛灉褰撳墠璇锋眰澹版槑浜?`Read` / `read_file` 杩欑被璇诲彇宸ュ叿锛屽吋瀹瑰眰浼氶澶栨敞鍏ヤ竴鏉?read-tool cache guard锛氬綋璇诲彇缁撴灉鍙〃绀衡€滄枃浠舵湭鍙樻洿 / 宸插湪鍘嗗彶涓?/ 璇峰紩鐢ㄥ厛鍓嶄笂涓嬫枃 / 娌℃湁姝ｆ枃鍐呭鈥濇椂锛屾ā鍨嬪繀椤绘妸瀹冭涓哄唴瀹逛笉鍙敤锛屼笉鑳藉弽澶嶈皟鐢ㄥ悓涓€涓棤姝ｆ枃璇诲彇锛涘簲鏀逛负璇锋眰瀹屾暣姝ｆ枃璇诲彇鑳藉姏锛屾垨鍚戠敤鎴疯鏄庨渶瑕侀噸鏂版彁渚涙枃浠跺唴瀹广€傝繖涓害鏉熷彧缂撹В瀹㈡埛绔紦瀛樿繑鍥炵┖鍐呭瀵艰嚧鐨勬寰幆锛孌S2API 涓嶄細涔熸棤娉曞嚟绌烘仮澶嶅鎴风鏈湴鏂囦欢姝ｆ枃銆?
OpenAI 璺緞瀹炵幇锛?[internal/promptcompat/tool_prompt.go](../internal/promptcompat/tool_prompt.go)

Claude 璺緞瀹炵幇锛?[internal/httpapi/claude/handler_utils.go](../internal/httpapi/claude/handler_utils.go)

缁熶竴宸ュ叿璋冪敤鏍煎紡妯℃澘锛?[internal/toolcall/tool_prompt.go](../internal/toolcall/tool_prompt.go)

杩欎篃鏄」鐩€滅綉椤靛璇濈函鏂囨湰鍏煎鈥濈殑鍏抽敭璁捐锛?
- tools 瀵逛笅娓告潵璇达紝鏈川涓婃槸 prompt 鍐呰鍒?- 涓嶆槸 native tool schema transport

## 7. assistant 鐨?tool_calls / reasoning 濡備綍淇濈暀

### 7.1 reasoning 淇濈暀鏂瑰紡

assistant 鐨?reasoning 浼氬彉鎴愪竴涓樉寮忔爣绛惧潡锛?
```text
[reasoning_content]
...
[/reasoning_content]
```

鐒跺悗鍐嶆帴鍙鍥炵瓟姝ｆ枃銆?
瀵规渶缁堣繑鍥炵粰瀹㈡埛绔殑 assistant 杞锛宺easoning 涓嶄細鍥犱负鏈疆杈撳嚭浜嗗伐鍏疯皟鐢ㄨ€岃涓㈠純銆侽penAI Chat 浼氬湪鍚屼竴涓?assistant message 涓婂悓鏃惰繑鍥?`reasoning_content` 鍜?`tool_calls`锛汷penAI Responses 浼氬厛杩斿洖涓€涓寘鍚?`reasoning` content 鐨?assistant message item锛屽啀杩斿洖鍚庣画 `function_call` item锛汣laude / Gemini 涔熶細鍦ㄥ悇鑷師鐢?thinking / thought 缁撴瀯鍚庣户缁繑鍥?tool_use / functionCall銆?
瀵硅繘鍏ュ悗缁?prompt / `chat_context.txt` 鐨勫巻鍙茶疆娆★紝鍏煎灞備篃浼氭妸鍚屼竴杞伐鍏疯皟鐢ㄥ墠鐨?reasoning 缁戝畾鍒?assistant tool call 鍘嗗彶涓娿€侽penAI Chat 鍘熺敓 `reasoning_content + tool_calls` 浼氱洿鎺ヤ繚鐣欙紱OpenAI Responses 鑻ヤ互 `reasoning` message item 鍚庢帴 `function_call` item 鐨勫舰寮忓洖鏀惧巻鍙诧紝浼氬湪褰掍竴鍖栨椂鍚堝苟涓哄悓涓€涓?assistant 鍘嗗彶鍧楋紱Claude 鐨?`thinking` block 浼氱粦瀹氬埌鍚庣画 `tool_use`锛汫emini 鐨?`thought: true` part 浼氱粦瀹氬埌鍚庣画 `functionCall`銆傛渶缁?prompt 涓殑椤哄簭鍥哄畾涓?`[reasoning_content]...[/reasoning_content]`锛屽啀鎺?DSML tool call 澶栧３銆?
### 7.2 鍘嗗彶 tool_calls 淇濈暀鏂瑰紡

assistant 鍘嗗彶 `tool_calls` 涓嶄細淇濈暀鎴?OpenAI 鍘熺敓 JSON锛岃€屼細杞垚 prompt 鍙鐨?DSML 澶栧３锛?
```xml
<|DSML|tool_calls>
  <|DSML|invoke name="read_file">
    <|DSML|parameter name="path"><![CDATA[src/main.go]]></|DSML|parameter>
  </|DSML|invoke>
</|DSML|tool_calls>
```

濡傛灉瀹㈡埛绔巻鍙查噷娌℃湁缁撴瀯鍖?`tool_calls` 瀛楁銆佸嵈鎶婁竴涓彲鐙珛瑙ｆ瀽鐨?assistant 宸ュ叿鍧楁斁杩涗簡鏅€?`content`锛屽吋瀹瑰眰浼氬湪鍐欏叆鍚庣画 prompt 鍓嶅厛鎸夊伐鍏疯皟鐢ㄨВ鏋愬畠锛屽啀閲嶆覆鏌撲负瑙勮寖 DSML 鍘嗗彶澶栧３銆傝繖鏍峰彲浠ラ伩鍏嶄竴娆?malformed 宸ュ叿鍧楁湭琚粨鏋勫寲淇濆瓨鍚庯紝浣滀负鏅€?assistant 鏂囨湰鍥炵亴锛岀户缁薄鏌撳悗缁ā鍨嬬殑 few-shot 宸ュ叿鏍煎紡銆?
瑙ｆ瀽灞傚悓鏃跺吋瀹规棫寮忕函 XML 褰㈡€侊細`<tool_calls>` / `<invoke>` / `<parameter>`銆備袱鑰呴兘浼氬厛褰掍竴鍒扮幇鏈?XML 瑙ｆ瀽璇箟锛涘叾浠栨棫鏍煎紡閮戒細浣滀负鏅€氭枃鏈繚鐣欙紝涓嶄細浣滀负鍙墽琛岃皟鐢ㄨ娉曘€?渚嬪鏄?parser 浼氬涓€涓潪甯哥獎鐨勬ā鍨嬪け璇仛淇锛氬鏋?assistant 杈撳嚭浜?`<invoke ...>` ... `</tool_calls>`锛堟垨 DSML 瀵瑰簲鏍囩锛夛紝浣嗘紡鎺夋渶鍓嶉潰鐨?opening wrapper锛岃В鏋愰樁娈典細鍦?wrapper-confidence 瓒冲楂樻椂琛ュ洖 wrapper 鍚庡啀灏濊瘯璇嗗埆銆傝繖閲岀殑 wrapper-confidence 鎸?scanner 宸茬粡璇嗗埆鍑虹櫧鍚嶅崟宸ュ叿澹崇粨鏋勶紝鍓╀綑澶辫触鍙儚澹冲眰缁撴瀯婕傜Щ锛岃€屼笉鏄涔変笂鎺ヨ繎浣嗕笉鍦ㄧ櫧鍚嶅崟鍐呯殑 near-miss 鏍囩鍚嶃€備慨澶嶆垚鍔熸椂锛寃rapper 鍚庨潰鐨?suffix prose 浼氱户缁繚鐣欏湪鍙鏂囨湰閲岋紱淇澶辫触鏃讹紝璇ュ潡浠嶆寜鏅€氭枃鏈鐞嗐€?
杩欎欢浜嬪緢閲嶈锛屽洜涓哄畠鍐冲畾浜嗭細

- 鍘嗗彶宸ュ叿璋冪敤鍦?prompt 涓槸鈥滃彲瑙佹枃鏈巻鍙测€?- 涓嶆槸鈥滈殣钘忕粨鏋勫寲鍏冩暟鎹€?
瀹炵幇浣嶇疆锛?[internal/prompt/tool_calls.go](../internal/prompt/tool_calls.go)

### 7.3 tool result 淇濈暀鏂瑰紡

tool / function role 鐨勭粨鏋滀細浣滀负 `<|Tool|>...<|end鈻乷f鈻乼oolresults|>` 杩涘叆 prompt銆?
濡傛灉 tool content 涓虹┖锛屽綋鍓嶄細琛ユ垚瀛楃涓?`"null"`锛岄伩鍏嶆暣涓?tool turn 涓㈠け銆?
## 8. files銆侀檮浠躲€乻ystemprompt 鏂囦欢鐨勫疄闄呰涔?
杩欓噷瑕佹槑纭尯鍒嗕袱绫讳笢瑗匡細

1. 鏂囨湰鍨?system prompt
   渚嬪 OpenAI `developer` / `system` / Responses `instructions` / Claude top-level `system`
   杩欑被浼氳繘鍏?`prompt`銆?2. 鏂囦欢鍨?systemprompt
   渚嬪閫氳繃闄勪欢銆乣input_file`銆乥ase64銆乨ata URL 涓婁紶鐨勬枃浠?   杩欑被涓嶄細鐩存帴鍐呰仈杩?`prompt`锛岃€屾槸杩涘叆 `ref_file_ids`銆?
OpenAI 鏂囦欢鐩稿叧瀹炵幇锛?
- inline/base64/data URL 涓婁紶锛?  [internal/httpapi/openai/files/file_inline_upload.go](../internal/httpapi/openai/files/file_inline_upload.go)
- 鏂囦欢 ID 鏀堕泦锛?  [internal/promptcompat/file_refs.go](../internal/promptcompat/file_refs.go)

OpenAI 鐨勬枃浠朵笂浼犵幇鍦ㄤ笉鍐嶆槸鈥滃彧浼犳枃浠舵湰浣撯€濈殑閫氱敤璺緞锛岃€屾槸浼氬厛鏍规嵁璇锋眰閲岀殑 `model` 瑙ｆ瀽鍑?DeepSeek 鐨勪笂浼犵被鍨嬶紝骞舵妸瀹冮€忎紶鍒颁笂浼犳帴鍙ｇ殑 `x-model-type`銆傚綋鍓嶅彲瑙佺殑涓婁紶绫诲瀷灏辨槸 `default` / `expert` / `vision`锛屽叾涓?vision 璇锋眰涓婁紶鍥剧墖鏃跺繀椤诲甫涓?`vision`锛屽惁鍒欎笅娓稿鏄撻€€鍥炲埌浠呮枃鏈垨 OCR 璇箟銆傝繖涓ā鍨嬬被鍨嬩細鍚屾椂鐢ㄤ簬锛?
- `/v1/files` 杩欑被鐙珛鏂囦欢涓婁紶鍏ュ彛
- Chat / Responses 鐨?inline 鍥剧墖銆侀檮浠朵笂浼?- current input file 瑙﹀彂鏃剁敓鎴愮殑 `chat_context.txt` 涓婁笅鏂囨枃浠?
涔熷氨鏄锛屾枃浠朵笂浼犲拰瀹屾垚璇锋眰鐨?`model_type` 鐜板湪鏄竴鑷寸殑锛氬畬鎴?payload 閲屼粛鐒舵槸 `model_type`锛屼笂浼犳枃浠跺垯浼氬湪 DeepSeek 涓婁紶闃舵鎼哄甫鍚屾牱鐨勬ā鍨嬬被鍨嬩俊鎭€?
缁撹锛?
- 鈥渟ystemprompt 鏂囧瓧鈥濆湪 prompt 閲?- 鈥渟ystemprompt 鏂囦欢鈥濋€氬父鍙湪 `ref_file_ids` 閲?
闄ら潪璋冪敤鏂硅嚜宸辨妸鏂囦欢鍐呭灞曞紑鍚庡啀濉炶繘 system/developer 鏂囨湰锛屽惁鍒欐枃浠跺唴瀹逛笉浼氳嚜鍔ㄥ嚭鐜板湪 prompt 姝ｆ枃銆?
## 9. 澶氳疆鍘嗗彶涓轰粈涔堜笉浼氫竴鐩村畬鏁村唴鑱斿湪 prompt

鍏煎灞傜幇鍦ㄥ彧淇濈暀 `current_input_file` 杩欎竴绉嶆媶鍒嗘柟寮忥紱鏃х殑 `history_split` 閰嶇疆瀛楁宸茬Щ闄わ紝璇诲彇鏃ч厤缃椂浼氬拷鐣ュ畠涓斾笉浼氬啀鍐欏洖銆?
- `current_input_file` 榛樿寮€鍚紱瀹冨湪缁熶竴 completion runtime 鍏ュ彛鍏ㄥ眬鐢熸晥锛岀敤浜庢妸鈥滃畬鏁翠笂涓嬫枃鈥濆悎骞惰繘 `chat_context.txt` 涓婁笅鏂囨枃浠躲€傚綋鏈€鏂?user turn 鐨勭函鏂囨湰闀垮害杈惧埌 `current_input_file.min_chars`锛堥粯璁?`0`锛夋椂锛宺untime 浼氫笂浼犱竴涓枃浠跺悕涓?`chat_context.txt` 鐨勪笂涓嬫枃鏂囦欢銆傛枃浠跺唴瀹逛細鍏堢粡杩囧悇鍗忚鍏ュ彛鐨勬爣鍑嗗寲锛屽啀搴忓垪鍖栨垚鎸夎疆娆＄紪鍙风殑 `chat_context.txt` 椋庢牸 transcript锛屽甫鏈?`# chat_context.txt` 鏍囬鍜?`=== N. ROLE ===` 鍒嗘锛涘鏋滃綋鍓嶈姹傚０鏄庝簡鍙敤宸ュ叿锛岃繕浼氭妸宸ュ叿鍚嶇О銆佹弿杩板拰鍙傛暟 schema 鍗曠嫭涓婁紶鎴?`tool_schema.txt`锛屽甫鏈?`# tool_schema.txt` 鏍囬銆俵ive prompt 涓垯浼氱粰鍑轰竴涓?continuation 璇皵鐨?user 娑堟伅锛屽紩瀵兼ā鍨嬩粠 `chat_context.txt` 鐨勬渶鏂扮姸鎬佺户缁帹杩涳紝骞跺湪鏈夊伐鍏锋枃浠舵椂鏄庣‘鍙敤宸ュ叿 schema 浣嶄簬 `tool_schema.txt`锛泂ystem prompt 涔熶細鍦ㄧ粺涓€ DSML 宸ュ叿鏍煎紡绾︽潫鍓嶈鏄?`tool_schema.txt` 鏄彲璋冪敤宸ュ叿鍜?schema 鐨勬潈濞佹潵婧愶紝鍚屾椂淇濈暀鏈疆宸ュ叿閫夋嫨绛栫暐锛岄伩鍏嶆妸浠诲姟鎷夊洖璧风偣銆?- 濡傛灉 `current_input_file.enabled=false`锛岃姹備細鐩存帴閫忎紶锛屼笉涓婁紶浠讳綍鎷嗗垎涓婁笅鏂囨枃浠躲€?- 鍗充娇瑙﹀彂 `current_input_file` 鍚?live prompt 琚缉鐭紝瀵瑰鎴风鍥炲寘閲岀殑涓婁笅鏂?token 缁熻锛屼粛浼氭部鐢?*鎷嗗垎鍓嶇殑瀹屾暣 prompt 璇箟**鍋氳鏁帮紝鑰屼笉鏄寜缂╃煭鍚庣殑鍗犱綅 prompt 璁＄畻锛涘惁鍒欎細鎶婄湡瀹炰笂涓嬫枃鏄捐憲绠楀皬銆?
鐩稿叧瀹炵幇锛?
- 閰嶇疆璁块棶鍣細
  [internal/config/store_accessors.go](../internal/config/store_accessors.go)
- 褰撳墠杈撳叆杞枃浠讹細
  [internal/httpapi/openai/history/current_input_file.go](../internal/httpapi/openai/history/current_input_file.go)
- 鍏ㄥ眬 completion runtime 搴旂敤鐐癸細
  [internal/completionruntime/nonstream.go](../internal/completionruntime/nonstream.go)

褰撳墠杈撳叆杞枃浠跺惎鐢ㄥ苟瑙﹀彂鏃讹紝涓婁紶鐨勫巻鍙叉枃浠剁湡瀹炴枃浠跺悕鏄?`chat_context.txt`锛屾枃浠跺唴瀹规槸瀹屾暣 `messages` 涓婁笅鏂囷紱瀹冧細浣跨敤 OpenAI-compatible 鐨勬秷鎭?transcript 搴忓垪鍖栬鍒欏拰 DeepSeek 瑙掕壊鏍囪锛屽啀鎸夎疆娆＄紪鍙锋垚 `chat_context.txt` 椋庢牸鐨?transcript锛堜笉鍐嶆敞鍏ユ枃浠惰竟鐣屾爣绛撅級锛?
```text
[uploaded filename]: chat_context.txt
# chat_context.txt
Prior conversation history and tool progress.

=== 1. SYSTEM ===
...

=== 2. USER ===
...

=== 3. ASSISTANT ===
...

=== 4. TOOL ===
...
```

濡傛灉褰撳墠璇锋眰甯︽湁宸ュ叿锛宺untime 鍚屾椂涓婁紶 `tool_schema.txt`锛?
```text
[uploaded filename]: tool_schema.txt
# tool_schema.txt
Available tool descriptions and parameter schemas for this request.

You have access to these tools:

Tool: ...
Description: ...
Parameters: ...
```

寮€鍚悗锛岃姹傜殑 live prompt 涓嶅啀鐩存帴鍐呰仈瀹屾暣涓婁笅鏂囷紝涔熶笉鍐嶅唴鑱斿ぇ娈靛伐鍏?schema锛涘畠淇濈暀涓€涓?user role 鐨勭煭鎻愮ず锛屾彁绀烘ā鍨嬪熀浜庡凡鎻愪緵涓婁笅鏂囩洿鎺ュ洖绛旀渶鏂拌姹傦紝骞跺湪鏈夊伐鍏锋椂寮曠敤 `tool_schema.txt`銆備笂浼犲悗鐨?`chat_context.txt` file_id 浼氭帓鍦?`ref_file_ids` 鏈€鍓嶏紱濡傛灉瀛樺湪 `tool_schema.txt`锛屽畠鐨?file_id 绱ч殢鍏跺悗锛涘鎴风宸叉湁鐨勫叾浠?file_id 淇濇寔鍦ㄥ悗闈€備笂涓嬫枃 token 缁熻浼氬寘鍚笂浼犵殑鍘嗗彶鏂囦欢銆佸伐鍏锋枃浠跺拰 live prompt銆傝嚜鍔ㄧ敓鎴愮殑 current-input 鏂囦欢寮曠敤浼氳璁板綍涓?runtime 鐘舵€侊紱濡傛灉鎵樼璐﹀彿妯″紡鍒囧彿 fresh retry锛宺untime 浼氶噸鏂颁笂浼犺繖浜涜嚜鍔ㄦ枃浠讹紝鑰屼笉鏄妸涓婁竴璐﹀彿鐨?file_id 浜ょ粰鏂拌处鍙枫€?
## 10. 鍚勫崗璁叆鍙ｇ殑宸紓

### 10.1 OpenAI Chat / Responses

鐗圭偣锛?
- `developer` 浼氭槧灏勫埌 `system`
- Responses `instructions` 浼?prepend 涓?system message
- 鏅€氱洿浼犳椂 `tools` 浼氭敞鍏?system prompt锛沗current_input_file` 瑙﹀彂鏃跺伐鍏锋弿杩?schema 浼氭媶鎴?`tool_schema.txt`锛宻ystem prompt 淇濈暀鏍煎紡/绛栫暐瑙勫垯骞舵槑纭姹傛ā鍨嬩粠 `tool_schema.txt` 鑾峰彇鍙皟鐢ㄥ伐鍏峰拰 schema
- `attachments` / `input_file` / inline 鏂囦欢浼氳繘鍏?`ref_file_ids`
- current input file 鍦ㄧ粺涓€ completion runtime 鍏ュ彛鍏ㄥ眬鐢熸晥

### 10.2 Claude Messages

鐗圭偣锛?
- top-level `system` 浼樺厛浣滀负绯荤粺鎻愮ず
- `tool_use` / `tool_result` 浼氳杞崲鎴愮粺涓€鐨?assistant/tool 鍘嗗彶璇箟
- 鏅€氱洿浼犳椂 `tools` 鍚屾牱浼氳骞惰繘 system prompt锛沗current_input_file` 瑙﹀彂鏃朵細娌跨敤缁熶竴鐨?`tool_schema.txt` 鎷嗗垎涓婁紶璺緞
- 甯歌鎵ц閫氳繃 `internal/httpapi/claude/handler_messages.go` 杞埌 OpenAI chat 璺緞锛屾ā鍨?alias 浼氬厛瑙ｆ瀽鎴?DeepSeek 鍘熺敓妯″瀷
- 褰撳墠浠ｇ爜閲屾病鏈夊儚 OpenAI 閭ｆ牱瀹屾暣鐨?`ref_file_ids` 闄勪欢閾捐矾

### 10.3 Gemini

鐗圭偣锛?
- `systemInstruction`銆乣contents.parts`銆乣functionCall`銆乣functionResponse` 浼氬厛褰掍竴
- tools 浼氳浆鎴?OpenAI 椋庢牸 function schema
- prompt 鏋勫缓澶嶇敤 OpenAI 鐨?`promptcompat.BuildOpenAIPromptForAdapter`锛宍current_input_file` 瑙﹀彂鏃朵篃浼氫娇鐢ㄧ粺涓€鐨?`tool_schema.txt` 鎷嗗垎涓婁紶璺緞
- 鏈瘑鍒殑闈炴枃鏈?part 浼氳瀹夊叏搴忓垪鍖栬繘 prompt锛屽苟瀵逛簩杩涘埗/鐤戜技 base64 鍐呭鍋氱渷鐣ユ垨鎴柇澶勭悊

涔熷氨鏄锛孏emini 鍦ㄢ€滄渶缁?prompt 璇箟鈥濅笂锛屽敖閲忓拰 OpenAI 淇濇寔涓€鑷淬€?
## 11. 涓€浠借创杩戠湡瀹炵殑鏈€缁堜笂涓嬫枃绀烘剰

鍋囪鐢ㄦ埛鍙戞潵涓€涓杞姹傦細

- 鏈?system/developer 鏂囨湰
- 鏈?tools
- 鏈変竴涓枃浠跺瀷 systemprompt 闄勪欢
- 鏈夊巻鍙?assistant tool call / tool result
- current input file 宸茶Е鍙?
閭ｄ箞鏈€缁堜笂涓嬫枃鏇存帴杩戯細

```json
{
  "prompt": "<|begin鈻乷f鈻乻entence|><|System|>鍘?system / developer\n\nTOOL CALL FORMAT 鈥?FOLLOW EXACTLY: ...<|end鈻乷f鈻乮nstructions|><|User|>Continue from the latest state in the attached chat_context.txt context. Treat it as the current working state and answer the latest user request directly. Available tool descriptions and parameter schemas are attached in tool_schema.txt; use only those tools and follow the tool-call format rules in this prompt.<|Assistant|>",
  "ref_file_ids": [
    "file-ds2api-history",
    "file-ds2api-tools",
    "file-systemprompt",
    "file-other-attachment"
  ],
  "thinking_enabled": true,
  "search_enabled": false
}
```

杩欐鏄€淎PI 杞綉椤靛璇濈函鏂囨湰鈥濈殑鏍稿績鎴愭灉锛?
- 澶ч儴鍒嗙粨鏋勫寲璇箟琚帇杩?`prompt`
- 鏂囦欢淇濇寔鏂囦欢
- 闇€瑕佹椂鎶婂畬鏁翠笂涓嬫枃鎷嗚繘 `chat_context.txt` 涓婁笅鏂囨枃浠讹紝骞舵寜杞缂栧彿鎴?transcript

## 12. 淇敼鏃跺繀椤诲悓姝ユ湰鏂囨。鐨勫満鏅?
鍙瑙︾浠ヤ笅浠讳竴绫昏涓猴紝灏卞繀椤诲湪鍚屼竴鎻愪氦鎴栧悓涓€ PR 涓洿鏂版湰鏂囨。锛?
- 瑙掕壊鏄犲皠鍙樻洿
- system / developer / instructions 鍚堝苟瑙勫垯鍙樻洿
- assistant reasoning 淇濈暀鏍煎紡鍙樻洿
- assistant 鍘嗗彶 `tool_calls` 鐨?XML 鍛堢幇鏂瑰紡鍙樻洿
- tool result 娉ㄥ叆鏂瑰紡鍙樻洿
- tool prompt 妯℃澘鎴?tool_choice 绾︽潫鍙樻洿
- inline 鏂囦欢涓婁紶 / 鏂囦欢寮曠敤鏀堕泦瑙勫垯鍙樻洿
- current input file 瑙﹀彂鏉′欢銆佷笂浼犳牸寮忋€乣chat_context.txt` transcript 缁撴瀯鍙樻洿
- 鏃?`history_split` 瀛楁蹇界暐/娓呯悊琛屼负鍙樻洿
- completion payload 瀛楁璇箟鍙樻洿
- Claude / Gemini 瀵硅繖濂楃粺涓€璇箟鐨勫鐢ㄥ叧绯诲彉鏇?
浼樺厛妫€鏌ヨ繖浜涙枃浠讹細

- `internal/promptcompat/request_normalize.go`
- `internal/promptcompat/prompt_build.go`
- `internal/promptcompat/message_normalize.go`
- `internal/promptcompat/tool_prompt.go`
- `internal/httpapi/openai/files/file_inline_upload.go`
- `internal/promptcompat/file_refs.go`
- `internal/httpapi/openai/history/current_input_file.go`
- `internal/completionruntime/nonstream.go`
- `internal/promptcompat/responses_input_normalize.go`
- `internal/httpapi/claude/standard_request.go`
- `internal/httpapi/claude/handler_utils.go`
- `internal/httpapi/gemini/convert_request.go`
- `internal/httpapi/gemini/convert_messages.go`
- `internal/httpapi/gemini/convert_tools.go`
- `internal/prompt/messages.go`
- `internal/prompt/tool_calls.go`
- `internal/promptcompat/standard_request.go`

## 13. 寤鸿鐨勬渶灏忛獙璇?
鏀瑰姩杩欐潯閾捐矾鍚庯紝鑷冲皯琛ラ綈鎴栨鏌ヨ繖浜涙祴璇曪細

- `go test ./internal/prompt/...`
- `go test ./internal/httpapi/openai/...`
- `go test ./internal/httpapi/claude/...`
- `go test ./internal/httpapi/gemini/...`
- `go test ./internal/util/...`

濡傛灉鏀圭殑鏄?tool call 鐩稿叧鍏煎璇箟锛岃繕搴斿悓鏃舵鏌ワ細

- `go test ./internal/toolcall/...`
- `go test ./internal/toolstream/...`
- `./tests/scripts/run-unit-node.sh`

## 14. 鏂囨。鍚屾绾﹀畾

鏈枃妗ｆ槸杩欐潯鍏煎閾捐矾鐨勪笓椤硅鏄庛€?
濡傛灉澶栭儴鎺ュ彛琛屼负涔熷彉浜嗭紝杩樺簲鍚屾妫€鏌ワ細

- [API.md](../API.md)
- [API.en.md](../API.en.md)
- [docs/toolcall-semantics.md](./toolcall-semantics.md)

鍘熷垯鏄細

- 鍐呴儴涓婚摼璺彉鍖栵紝鑷冲皯鏇存柊鏈枃妗?- 澶栭儴鍙濂戠害鍙樺寲锛屽啀鍚屾鏇存柊 API 鏂囨。
