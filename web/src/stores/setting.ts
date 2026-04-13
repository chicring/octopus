import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type Locale = 'zh_hans' | 'zh_hant' | 'en';
export type ThemeStyle = 'default' | 'claude';

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
        }
    )
);
