'use client';

import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';

export type RankDimension = 'channel' | 'model' | 'key';
export type RankSortMode = 'cost' | 'count' | 'tokens';
export type ChartMetricType = 'cost' | 'count' | 'tokens' | 'tps';
export type ChartPeriod = '1' | '7' | '30';

interface HomeViewState {
    rankDimension: RankDimension;
    rankSortMode: RankSortMode;
    chartMetricType: ChartMetricType;
    chartPeriod: ChartPeriod;
    chartSelectedKeyIDs: number[]; // 空数组 = 全部密钥
    setRankDimension: (value: RankDimension) => void;
    setRankSortMode: (value: RankSortMode) => void;
    setChartMetricType: (value: ChartMetricType) => void;
    setChartPeriod: (value: ChartPeriod) => void;
    setChartSelectedKeyIDs: (value: number[]) => void;
}

export const useHomeViewStore = create<HomeViewState>()(
    persist(
        (set) => ({
            rankDimension: 'channel',
            rankSortMode: 'cost',
            chartMetricType: 'cost',
            chartPeriod: '1',
            chartSelectedKeyIDs: [],
            setRankDimension: (value) => set({ rankDimension: value }),
            setRankSortMode: (value) => set({ rankSortMode: value }),
            setChartMetricType: (value) => set({ chartMetricType: value }),
            setChartPeriod: (value) => set({ chartPeriod: value }),
            setChartSelectedKeyIDs: (value) => set({ chartSelectedKeyIDs: value }),
        }),
        {
            name: 'home-view-options-storage',
            storage: createJSONStorage(() => localStorage),
            partialize: (state) => ({
                rankDimension: state.rankDimension,
                rankSortMode: state.rankSortMode,
                chartMetricType: state.chartMetricType,
                chartPeriod: state.chartPeriod,
                chartSelectedKeyIDs: state.chartSelectedKeyIDs,
            }),
        }
    )
);
