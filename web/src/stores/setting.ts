import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type Locale = 'zh_hans' | 'zh_hant' | 'en';
export type ThemeStyle = 'default' | 'claude';

const VALID_LOCALES: Locale[] = ['zh_hans', 'zh_hant', 'en'];
const VALID_THEME_STYLES: ThemeStyle[] = ['default', 'claude'];

function isValidLocale(value: unknown): value is Locale {
    return typeof value === 'string' && VALID_LOCALES.includes(value as Locale);
}

function isValidThemeStyle(value: unknown): value is ThemeStyle {
    return typeof value === 'string' && VALID_THEME_STYLES.includes(value as ThemeStyle);
}

interface SettingState {
    locale: Locale;
    themeStyle: ThemeStyle;
    setLocale: (locale: Locale) => void;
    setThemeStyle: (style: ThemeStyle) => void;
}

export const useSettingStore = create<SettingState>()(
    persist(
        (set) => ({
            locale: 'zh_hans',
            themeStyle: 'default',
            setLocale: (locale) => set({ locale }),
            setThemeStyle: (themeStyle) => set({ themeStyle }),
        }),
        {
            name: 'octopus-settings',
            merge: (persisted, current) => {
                const typed = persisted as Partial<SettingState>;
                return {
                    ...current,
                    locale: isValidLocale(typed?.locale) ? typed.locale : current.locale,
                    themeStyle: isValidThemeStyle(typed?.themeStyle) ? typed.themeStyle : current.themeStyle,
                };
            },
        }
    )
);
