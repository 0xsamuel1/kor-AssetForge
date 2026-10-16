export interface FileValidationOptions {
  allowedTypes?: string[]
  maxSize?: number
  minSize?: number
  maxFiles?: number
}

export interface FileValidationResult {
  valid: boolean
  error?: string
}

export interface FileWithPreview extends File {
  preview?: string
  id: string
}

const DEFAULT_ALLOWED_TYPES = [
  "image/jpeg",
  "image/png",
  "image/gif",
  "image/webp",
  "image/svg+xml",
  "application/pdf",
  "application/msword",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
]

const DEFAULT_MAX_SIZE = 10 * 1024 * 1024

let fileCounter = 0

export function generateFileId(): string {
  return `file_${Date.now()}_${++fileCounter}`
}

export function validateFile(
  file: File,
  options: FileValidationOptions = {},
): FileValidationResult {
  const {
    allowedTypes = DEFAULT_ALLOWED_TYPES,
    maxSize = DEFAULT_MAX_SIZE,
    minSize = 0,
  } = options

  if (!allowedTypes.includes(file.type) && allowedTypes.length > 0) {
    return {
      valid: false,
      error: `File type ${file.type || "unknown"} is not supported. Allowed types: ${allowedTypes.map((t) => t.split("/")[1]).join(", ")}`,
    }
  }

  if (file.size > maxSize) {
    const maxSizeMB = (maxSize / (1024 * 1024)).toFixed(1)
    return {
      valid: false,
      error: `File size exceeds ${maxSizeMB}MB limit.`,
    }
  }

  if (file.size < minSize) {
    return {
      valid: false,
      error: "File is too small.",
    }
  }

  return { valid: true }
}

export function validateFiles(
  files: File[],
  options: FileValidationOptions = {},
): FileValidationResult {
  const { maxFiles = 10 } = options

  if (files.length > maxFiles) {
    return {
      valid: false,
      error: `You can upload up to ${maxFiles} files at once.`,
    }
  }

  for (const file of files) {
    const result = validateFile(file, options)
    if (!result.valid) return result
  }

  return { valid: true }
}

export function createFilePreviews(files: File[]): FileWithPreview[] {
  return files.map((file) => {
    const fileWithPreview = Object.assign(file, {
      id: generateFileId(),
    }) as FileWithPreview

    if (file.type.startsWith("image/")) {
      fileWithPreview.preview = URL.createObjectURL(file)
    }

    return fileWithPreview
  })
}

export function revokeFilePreviews(files: FileWithPreview[]): void {
  files.forEach((file) => {
    if (file.preview) {
      URL.revokeObjectURL(file.preview)
    }
  })
}

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return "0 Bytes"
  const k = 1024
  const sizes = ["Bytes", "KB", "MB", "GB"]
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

export function isImageFile(file: File): boolean {
  return file.type.startsWith("image/")
}

export function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}
