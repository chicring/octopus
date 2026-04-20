'use client';

import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';

export type RankDimension = 'channel' | 'model';
export type RankSortMode = 'cost' | 'count' | 'tokens';
export type ChartMetricType = 'cost' | 'count' | 'tokens' | 'tps';
export type ChartPeriod = '1' | '7' | '30';

interface HomeViewState {
    rankDimension: RankDimension;
    rankSortMode: RankSortMode;
    chartMetricType: ChartMetricType;
    chartPeriod: ChartPeriod;
    setRankDimension: (value: RankDimension) => void;
    setRankSortMode: (value: RankSortMode) => void;
    setChartMetricType: (value: ChartMetricType) => void;
    setChartPeriod: (value: ChartPeriod) => void;
}

export const useHomeViewStore = create<HomeViewState>()(
    persist(
        (set) => ({
            rankDimension: 'channel',
            rankSortMode: 'cost',
            chartMetricType: 'cost',
            chartPeriod: '1',
            setRankDimension: (value) => set({ rankDimension: value }),
            setRankSortMode: (value) => set({ rankSortMode: value }),
            setChartMetricType: (value) => set({ chartMetricType: value }),
            setChartPeriod: (value) => set({ chartPeriod: value }),
        }),
        {
            name: 'home-view-options-storage',
            storage: createJSONStorage(() => localStorage),
            partialize: (state) => ({
                rankDimension: state.rankDimension,
                rankSortMode: state.rankSortMode,
                chartMetricType: state.chartMetricType,
                chartPeriod: state.chartPeriod,
            }),
        }
    )
);
