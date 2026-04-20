// Minimal RHF integration helpers. Wraps shadcn Input/Label/etc so callers
// don't repeat the same boilerplate for every form field.
//
// Usage:
//   const form = useForm({ resolver: zodResolver(schema) })
//   <FormProvider {...form}>
//     <FormField name="email" label="이메일">
//       <Input {...form.register('email')} />
//     </FormField>
//   </FormProvider>

import type { ReactNode } from 'react'
import { type FieldValues, useFormContext, type Path } from 'react-hook-form'

import { Label } from '@/components/ui/label'

interface FormFieldProps<T extends FieldValues> {
  name: Path<T>
  label: string
  required?: boolean
  description?: string
  children: ReactNode
}

export function FormField<T extends FieldValues>({
  name,
  label,
  required,
  description,
  children,
}: FormFieldProps<T>) {
  const {
    formState: { errors },
  } = useFormContext<T>()

  // RHF nests errors by field name; we only handle the top-level path here.
  // Nested objects need to be addressed at the call site.
  const error = errors[name as string]
  const errorMessage = error?.message as string | undefined

  return (
    <div className="space-y-1">
      <Label htmlFor={String(name)}>
        {label}
        {required && <span className="ml-1 text-destructive">*</span>}
      </Label>
      {children}
      {description && !errorMessage && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
      {errorMessage && <p className="text-xs text-destructive">{errorMessage}</p>}
    </div>
  )
}
