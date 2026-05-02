'use client';

import { useState } from 'react';
import { useCreateModel } from '@/api/endpoints/model';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Field, FieldLabel, FieldGroup } from '@/components/ui/field';
import {
    MorphingDialogClose,
    MorphingDialogTitle,
    MorphingDialogDescription,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { useTranslations } from 'next-intl';

export function CreateDialogContent() {
    const { setIsOpen } = useMorphingDialog();
    const t = useTranslations('model.create');
    const createModel = useCreateModel();

    const [formData, setFormData] = useState({
        name: '',
        input: '',
        output: '',
        cache_read: '',
        cache_write: '',
        context_length: '',
        max_output_tokens: '',
    });

    const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!formData.name.trim()) return;

        createModel.mutate({
            name: formData.name.trim(),
            input: parseFloat(formData.input) || 0,
            output: parseFloat(formData.output) || 0,
            cache_read: parseFloat(formData.cache_read) || 0,
            cache_write: parseFloat(formData.cache_write) || 0,
            context_length: parseInt(formData.context_length) || 0,
            max_output_tokens: parseInt(formData.max_output_tokens) || 0,
        }, {
            onSuccess: () => {
                setFormData({ name: '', input: '', output: '', cache_read: '', cache_write: '', context_length: '', max_output_tokens: '' });
                setIsOpen(false);
            }
        });
    };

    return (
        <div className="w-screen max-w-full md:max-w-xl">
            <MorphingDialogTitle>
                <header className="mb-5 flex items-center justify-between">
                    <h2 className="text-2xl font-bold text-card-foreground">{t('title')}</h2>
                    <MorphingDialogClose
                        className="relative right-0 top-0"
                        variants={{
                            initial: { opacity: 0, scale: 0.8 },
                            animate: { opacity: 1, scale: 1 },
                            exit: { opacity: 0, scale: 0.8 },
                        }}
                    />
                </header>
            </MorphingDialogTitle>
            <MorphingDialogDescription>
                <form onSubmit={handleSubmit}>
                    <FieldGroup className="gap-4">
                        <Field>
                            <FieldLabel htmlFor="model-name">{t('name')}</FieldLabel>
                            <Input
                                id="model-name"
                                value={formData.name}
                                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                                className="rounded-xl"
                            />
                        </Field>
                        <div className="grid grid-cols-2 gap-4">
                            <Field>
                                <FieldLabel htmlFor="model-input">{t('input')}</FieldLabel>
                                <Input
                                    id="model-input"
                                    type="number"
                                    step="any"
                                    value={formData.input}
                                    onChange={(e) => setFormData({ ...formData, input: e.target.value })}
                                    className="rounded-xl"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-output">{t('output')}</FieldLabel>
                                <Input
                                    id="model-output"
                                    type="number"
                                    step="any"
                                    value={formData.output}
                                    onChange={(e) => setFormData({ ...formData, output: e.target.value })}
                                    className="rounded-xl"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-cache-read">{t('cacheRead')}</FieldLabel>
                                <Input
                                    id="model-cache-read"
                                    type="number"
                                    step="any"
                                    value={formData.cache_read}
                                    onChange={(e) => setFormData({ ...formData, cache_read: e.target.value })}
                                    className="rounded-xl"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-cache-write">{t('cacheWrite')}</FieldLabel>
                                <Input
                                    id="model-cache-write"
                                    type="number"
                                    step="any"
                                    value={formData.cache_write}
                                    onChange={(e) => setFormData({ ...formData, cache_write: e.target.value })}
                                    className="rounded-xl"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-context-length">{t('contextLength')}</FieldLabel>
                                <Input
                                    id="model-context-length"
                                    type="number"
                                    step="1"
                                    value={formData.context_length}
                                    onChange={(e) => setFormData({ ...formData, context_length: e.target.value })}
                                    className="rounded-xl"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-max-output-tokens">{t('maxOutputTokens')}</FieldLabel>
                                <Input
                                    id="model-max-output-tokens"
                                    type="number"
                                    step="1"
                                    value={formData.max_output_tokens}
                                    onChange={(e) => setFormData({ ...formData, max_output_tokens: e.target.value })}
                                    className="rounded-xl"
                                />
                            </Field>
                        </div>
                        <Button
                            type="submit"
                            disabled={createModel.isPending || !formData.name.trim()}
                            className="w-full rounded-xl h-11"
                        >
                            {createModel.isPending ? t('submitting') : t('submit')}
                        </Button>
                    </FieldGroup>
                </form>
            </MorphingDialogDescription>
        </div>
    );
}
