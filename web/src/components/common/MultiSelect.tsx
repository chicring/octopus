'use client';

import { useState, useMemo } from 'react';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Check, ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

interface MultiSelectProps {
    options: string[];
    selected: string[];
    onSelectedChange: (values: string[]) => void;
    placeholder: string;
    searchPlaceholder?: string;
    emptyText?: string;
    selectAllText?: string;
    deselectAllText?: string;
}

export function MultiSelect({
    options,
    selected,
    onSelectedChange,
    placeholder,
    searchPlaceholder,
    emptyText,
    selectAllText,
    deselectAllText,
}: MultiSelectProps) {
    const [open, setOpen] = useState(false);
    const [search, setSearch] = useState('');

    const filteredOptions = useMemo(() => {
        if (!search.trim()) return options;
        const lower = search.toLowerCase();
        return options.filter((opt) => opt.toLowerCase().includes(lower));
    }, [options, search]);

    const allFilteredSelected = filteredOptions.length > 0 && filteredOptions.every((opt) => selected.includes(opt));

    const handleToggle = (value: string) => {
        if (selected.includes(value)) {
            onSelectedChange(selected.filter((v) => v !== value));
        } else {
            onSelectedChange([...selected, value]);
        }
    };

    const handleToggleAll = () => {
        if (allFilteredSelected) {
            onSelectedChange(selected.filter((v) => !filteredOptions.includes(v)));
        } else {
            const newSelected = new Set(selected);
            filteredOptions.forEach((opt) => newSelected.add(opt));
            onSelectedChange(Array.from(newSelected));
        }
    };

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <Button
                    variant="outline"
                    size="sm"
                    className={cn(
                        'min-h-11 gap-2 px-3 rounded-xl border-border',
                        selected.length > 0 && 'border-primary/30'
                    )}
                >
                    <span className="text-sm text-muted-foreground">{placeholder}</span>
                    {selected.length > 0 && (
                        <Badge variant="secondary" className="h-5 px-1.5 text-[10px]">
                            {selected.length}
                        </Badge>
                    )}
                    <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
                </Button>
            </PopoverTrigger>
            <PopoverContent
                className="w-56 p-0 rounded-xl"
                align="start"
            >
                <div className="p-2 pb-1">
                    <Input
                        placeholder={searchPlaceholder}
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        className="h-8 rounded-lg text-sm"
                    />
                </div>
                {filteredOptions.length > 0 && (
                    <div className="px-2 pb-1">
                        <button
                            onClick={handleToggleAll}
                            className="flex w-full items-center gap-2 rounded-lg px-2 py-1 text-xs text-muted-foreground hover:bg-muted/50"
                        >
                            <Checkbox
                                checked={allFilteredSelected}
                                className="size-3.5"
                            />
                            <span>{allFilteredSelected ? deselectAllText : selectAllText}</span>
                        </button>
                    </div>
                )}
                <div className="max-h-60 overflow-y-auto px-1 pb-1">
                    {filteredOptions.length === 0 ? (
                        <div className="py-6 text-center text-sm text-muted-foreground">
                            {emptyText}
                        </div>
                    ) : (
                        filteredOptions.map((option) => (
                            <label
                                key={option}
                                className={cn(
                                    'flex items-center gap-2 rounded-lg px-2 py-1.5 cursor-pointer text-sm',
                                    'hover:bg-muted/50',
                                    selected.includes(option) && 'bg-muted/20'
                                )}
                            >
                                <Checkbox
                                    checked={selected.includes(option)}
                                    onCheckedChange={() => handleToggle(option)}
                                    className="size-3.5"
                                />
                                <span className="truncate">{option}</span>
                            </label>
                        ))
                    )}
                </div>
            </PopoverContent>
        </Popover>
    );
}
