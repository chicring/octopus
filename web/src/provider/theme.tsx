"use client"

import * as React from "react"
import { ThemeProvider as NextThemesProvider, useTheme } from "next-themes"
import { useSettingStore } from "@/stores/setting"

// Claude 主题的 theme-color (Anthropic official tokens)
const CLAUDE_LIGHT_COLOR = "#F5F4ED"   // background-secondary light
const CLAUDE_DARK_COLOR = "#30302E"    // background-primary dark
// 默认主题的 theme-color
const DEFAULT_LIGHT_COLOR = "#eae9e3"
const DEFAULT_DARK_COLOR = "#413a2c"

function ThemeStyleApplier() {
    const { resolvedTheme } = useTheme()
    const themeStyle = useSettingStore((state) => state.themeStyle)

    // 应用主题风格到 html 元素
    React.useEffect(() => {
        const html = document.documentElement
        if (themeStyle === 'claude') {
            html.setAttribute('data-theme', 'claude')
        } else {
            html.removeAttribute('data-theme')
        }
    }, [themeStyle])

    // 更新 theme-color meta 标签
    React.useEffect(() => {
        const metaThemeColor = document.querySelector('meta[name="theme-color"]')
        if (metaThemeColor) {
            let color: string
            if (themeStyle === 'claude') {
                color = resolvedTheme === 'dark' ? CLAUDE_DARK_COLOR : CLAUDE_LIGHT_COLOR
            } else {
                color = resolvedTheme === 'dark' ? DEFAULT_DARK_COLOR : DEFAULT_LIGHT_COLOR
            }
            metaThemeColor.setAttribute('content', color)
        }
    }, [resolvedTheme, themeStyle])

    return null
}

export function ThemeProvider({ children, ...props }: React.ComponentProps<typeof NextThemesProvider>) {
    return (
        <NextThemesProvider {...props}>
            <ThemeStyleApplier />
            {children}
        </NextThemesProvider>
    )
}