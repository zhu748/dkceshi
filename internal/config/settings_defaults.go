package config

func init() {
	for _, spec := range []SettingSpec{
		{
			Key:      "responses",
			TitleKey: "settings.responsesTitle",
			Fields: []FieldSpec{
				{Name: "store_ttl_seconds", Kind: FieldInt, Default: 900, Min: 30, Max: 86400,
					LabelKey: "settings.responsesTTL"},
				{Name: "max_entries", Kind: FieldInt, Default: 256, Min: 1, Max: 4096, LabelKey: "settings.responsesMaxEntries"},
				{Name: "max_bytes_mib", Kind: FieldInt, Default: 128, Min: 1, Max: 1024, LabelKey: "settings.responsesMaxBytes"},
				{Name: "max_caller_entries", Kind: FieldInt, Default: 128, Min: 1, Max: 4096, LabelKey: "settings.responsesCallerEntries"},
			},
		},
		{
			Key: "tool_safety", TitleKey: "settings.toolSafetyTitle",
			Fields: []FieldSpec{
				{Name: "allow_reasoning_prose", Kind: FieldBool, Default: false, LabelKey: "settings.allowReasoningProse", HelpKey: "settings.allowReasoningProseHelp"},
			},
		},
		{
			Key:      "embeddings",
			TitleKey: "settings.embeddingsTitle",
			Fields: []FieldSpec{
				// Trim matches the accessor, which trims: the provider name is
				// used as-is by the embeddings call. An empty value means
				// "unset"; a value that only contains spaces is stored as that
				// same empty value, because a text input cannot tell the two
				// apart. NonBlank therefore only rejects a blank-but-not-empty
				// *stored* value, which a hand-edited config file can carry.
				{Name: "provider", Kind: FieldString, Default: "", Trim: true, NonBlank: true,
					LabelKey: "settings.embeddingsProvider"},
			},
		},
		{
			Key:            "thinking_injection",
			TitleKey:       "settings.thinkingInjectionTitle",
			DescriptionKey: "settings.thinkingInjectionDesc",
			Fields: []FieldSpec{
				{Name: "enabled", Kind: FieldBool, Default: false,
					LabelKey: "settings.fieldEnabled"},
				{Name: "prompt", Kind: FieldString, Default: "", Trim: true, Multiline: true,
					LabelKey: "settings.thinkingInjectionPrompt",
					HelpKey:  "settings.thinkingInjectionPromptHelp"},
			},
		},
		{
			Key:            "bottom_format_injection",
			TitleKey:       "settings.bottomFormatInjectionTitle",
			DescriptionKey: "settings.bottomFormatInjectionDesc",
			Fields: []FieldSpec{
				{Name: "enabled", Kind: FieldBool, Default: true,
					LabelKey: "settings.fieldEnabled"},
				{Name: "prompt", Kind: FieldString, Default: "", Trim: true, Multiline: true,
					LabelKey:       "settings.bottomFormatInjectionPrompt",
					HelpKey:        "settings.bottomFormatInjectionPromptHelp",
					PrefillDefault: true},
			},
		},
		{
			Key:            "input_file",
			TitleKey:       "settings.inputFileEnabled",
			DescriptionKey: "settings.inputFileDesc",
			Fields: []FieldSpec{
				{Name: "enabled", Kind: FieldBool, Default: false,
					LabelKey: "settings.fieldEnabled"},
				{Name: "min_chars", Kind: FieldInt, Default: 0, Min: 0, Max: 100000000,
					LabelKey: "settings.inputFileMinChars",
					HelpKey:  "settings.inputFileHelp"},
			},
		},
		// expert_prompt_segment is hidden: the settings page dropped the switch
		// when the feature was retired (the section component went away), and the
		// registry must keep it off the page the same way rather than letting the
		// generic renderer give it a card again. It stays registered so the config
		// keeps round-tripping and the settings API keeps reporting both fields.
		{
			Key:            "expert_prompt_segment",
			Hidden:         true,
			TitleKey:       "settings.expertPromptSegmentEnabled",
			DescriptionKey: "settings.expertPromptSegmentDesc",
			Fields: []FieldSpec{
				// Retired: the enable switch is forced off so a config file that
				// predates the retirement cannot turn segmentation back on.
				{Name: "enabled", Kind: FieldBool, Default: false, ForceValue: false,
					LabelKey: "settings.fieldEnabled"},
				{Name: "max_chars", Kind: FieldInt, Default: 160000, Min: 1000, Max: 100000000,
					LabelKey: "settings.expertPromptSegmentMaxChars",
					HelpKey:  "settings.expertPromptSegmentMaxCharsHelp"},
			},
		},
		// expert_text_file_inline is config-file only: the expert path reads it
		// but the settings page never exposed it, so registering it must not put
		// a control on the page.
		{
			Key:    "expert_text_file_inline",
			Hidden: true,
			Fields: []FieldSpec{
				{Name: "enabled", Kind: FieldBool, Default: true},
				{Name: "max_file_bytes", Kind: FieldInt, Default: 3 * 1024 * 1024, Min: 1, Max: 100 << 20},
			},
		},
		// auto_route_vision is retained for config compatibility. Retired, for the
		// same reason as expert_prompt_segment.
		{
			Key:    "auto_route_vision",
			Hidden: true,
			Fields: []FieldSpec{
				{Name: "enabled", Kind: FieldBool, Default: false, ForceValue: false},
			},
		},
		// tts is the master switch for /v1/audio/speech. It is visible on the
		// settings page and is also mirrored by the WebUI speech tab, which
		// reads and writes the same `tts.enabled` value.
		{
			Key:            "tts",
			TitleKey:       "settings.ttsTitle",
			DescriptionKey: "settings.ttsDesc",
			Fields: []FieldSpec{
				{Name: "enabled", Kind: FieldBool, Default: true,
					LabelKey: "settings.ttsEnabled",
					HelpKey:  "settings.ttsEnabledHelp"},
				{Name: "segment_chars", Kind: FieldInt, Default: 160, Min: 30, Max: 500,
					LabelKey: "settings.ttsSegmentChars",
					HelpKey:  "settings.ttsSegmentCharsHelp"},
				{Name: "multi_account", Kind: FieldBool, Default: true,
					LabelKey: "settings.ttsMultiAccount",
					HelpKey:  "settings.ttsMultiAccountHelp"},
				{Name: "account_concurrency", Kind: FieldBool, Default: true,
					LabelKey: "settings.ttsAccountConcurrency",
					HelpKey:  "settings.ttsAccountConcurrencyHelp"},
			},
		},
		{
			Key:            "keep_alive",
			TitleKey:       "settings.keepAliveTitle",
			DescriptionKey: "settings.keepAliveDesc",
			Fields: []FieldSpec{
				{Name: "enabled", Kind: FieldBool, Default: true,
					LabelKey: "settings.keepAliveEnabled",
					HelpKey:  "settings.keepAliveEnabledHelp"},
				{Name: "interval", Kind: FieldString, Default: "2-4", Trim: true,
					LabelKey: "settings.keepAliveInterval",
					HelpKey:  "settings.keepAliveIntervalHelp",
					Validate: ValidateKeepAliveInterval},
				{Name: "prompts", Kind: FieldString, Default: "现在纽约天气如何？;现在几点了？;你好啊;简单介绍一下你自己;9.8-9.11=？", Trim: true, Multiline: true,
					LabelKey: "settings.keepAlivePrompts",
					HelpKey:  "settings.keepAlivePromptsHelp"},
				{Name: "record_history", Kind: FieldBool, Default: false,
					LabelKey: "settings.keepAliveRecordHistory",
					HelpKey:  "settings.keepAliveRecordHistoryHelp"},
			},
		},
	} {
		RegisterSetting(spec)
	}
	// The settings-page cards, in the order they appear on the page: the two
	// injection switches sit with the TTL and provider they are configured next
	// to, keep-alive sits right below behavior, and the input-file split threshold
	// gets a card of its own. The remaining orders (1, 2 and 6) belong to the
	// section cards declared in section_admin.go, section_runtime.go and
	// section_auto_delete.go.
	for _, card := range []SettingCardSpec{
		{
			Key:      "behavior",
			TitleKey: "settings.behaviorTitle",
			Order:    3,
			Settings: []string{"responses", "embeddings", "thinking_injection", "bottom_format_injection", "tool_safety"},
		},
		{
			Key:            "keep_alive",
			TitleKey:       "settings.keepAliveTitle",
			DescriptionKey: "settings.keepAliveDesc",
			Order:          3,
			Settings:       []string{"keep_alive"},
		},
		{
			Key:            "input_file",
			TitleKey:       "settings.inputFileTitle",
			DescriptionKey: "settings.inputFileDesc",
			Order:          4,
			Settings:       []string{"input_file"},
		},
	} {
		RegisterSettingCard(card)
	}
}
