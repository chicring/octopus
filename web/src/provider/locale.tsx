'use client';

import { useEffect, useState, type ReactNode } from 'react';
import { NextIntlClientProvider } from 'next-intl';
import { useSettingStore, type Locale } from '@/stores/setting';

import zh_hansMessages from '../../public/locale/zh_hans.json';
import zh_hantMessages from '../../public/locale/zh_hant.json';
import enMessages from '../../public/locale/en.json';

const messages: Record<Locale, typeof zh_hansMessages> = {
    zh_hans: zh_hansMessages,
    zh_hant: zh_hantMessages,
    en: enMessages,
};

// next-intl 要求 BCP 47 格式的 locale 标签（如 zh-Hans），内部用下划线格式映射
const bcp47LocaleMap: Record<Locale, string> = {
    zh_hans: 'zh-Hans',
    zh_hant: 'zh-Hant',
    en: 'en',
};

export function LocaleProvider({ children }: { children: ReactNode }) {
    const { locale } = useSettingStore();
    const [currentLocale, setCurrentLocale] = useState<Locale>('zh_hans');

    useEffect(() => {
        setCurrentLocale(locale);
    }, [locale]);

    return (
        <NextIntlClientProvider
            locale={bcp47LocaleMap[currentLocale]}
            messages={messages[currentLocale]}
            timeZone="Asia/Shanghai"
        >
            {children}
        </NextIntlClientProvider>
    );
}

