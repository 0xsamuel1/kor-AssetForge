"use client"

import { useState, useCallback, useRef, useEffect } from "react"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { cn } from "@/lib/utils"
import {
  type FileWithPreview,
  type FileValidationOptions,
  validateFiles,
  createFilePreviews,
  revokeFilePreviews,
  formatFileSize,
  isImageFile,
} from "@/lib/file-utils"
import {
  Upload,
  X,
  File,
  ImageIcon,
  FileText,
  AlertCircle,
  CheckCircle2,
} from "lucide-react"

interface FileUploadProps {
  onFilesSelected: (files: FileWithPreview[]) => void
  onFilesRemoved?: (fileId: string) => void
  validationOptions?: FileValidationOptions
  maxFiles?: number
  accept?: string
  className?: string
  dropzoneText?: string
  disabled?: boolean
}

export function FileUpload({
  onFilesSelected,
  onFilesRemoved,
  validationOptions,
  maxFiles = 10,
  accept,
  className,
  dropzoneText = "Drag and drop files here, or click to browse",
  disabled = false,
}: FileUploadProps) {
  const [files, setFiles] = useState<FileWithPreview[]>([])
  const [isDragOver, setIsDragOver] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [uploadProgress, setUploadProgress] = useState<Record<string, number>>({})
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    return () => {
      revokeFilePreviews(files)
    }
  }, [files])

  const processFiles = useCallback(
    (incomingFiles: File[]) => {
      setError(null)

      const totalFiles = files.length + incomingFiles.length
      if (totalFiles > maxFiles) {
        setError(`You can upload up to ${maxFiles} files.`)
        return
      }

      const validation = validateFiles(incomingFiles, {
        maxFiles,
        ...validationOptions,
      })

      if (!validation.valid) {
        setError(validation.error || "Invalid file.")
        return
      }

      const filesWithPreviews = createFilePreviews(incomingFiles)
      const updatedFiles = [...files, ...filesWithPreviews]
      setFiles(updatedFiles)
      onFilesSelected(filesWithPreviews)

      filesWithPreviews.forEach((file) => {
        simulateProgress(file.id)
      })
    },
    [files, maxFiles, validationOptions, onFilesSelected],
  )

  const simulateProgress = (fileId: string) => {
    let progress = 0
    const interval = setInterval(() => {
      progress += Math.random() * 30
      if (progress >= 100) {
        progress = 100
        clearInterval(interval)
      }
      setUploadProgress((prev) => ({ ...prev, [fileId]: Math.min(progress, 100) }))
    }, 200)
  }

  const removeFile = useCallback(
    (fileId: string) => {
      const file = files.find((f) => f.id === fileId)
      if (file?.preview) URL.revokeObjectURL(file.preview)

      const updatedFiles = files.filter((f) => f.id !== fileId)
      setFiles(updatedFiles)
      setUploadProgress((prev) => {
        const next = { ...prev }
        delete next[fileId]
        return next
      })
      onFilesRemoved?.(fileId)
    },
    [files, onFilesRemoved],
  )

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragOver(true)
  }, [])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragOver(false)
  }, [])

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      e.stopPropagation()
      setIsDragOver(false)

      const droppedFiles = Array.from(e.dataTransfer.files)
      if (droppedFiles.length > 0) processFiles(droppedFiles)
    },
    [processFiles],
  )

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const selectedFiles = Array.from(e.target.files || [])
      if (selectedFiles.length > 0) processFiles(selectedFiles)
      e.target.value = ""
    },
    [processFiles],
  )

  const getFileIcon = (file: FileWithPreview) => {
    if (file.preview) {
      return (
        <img
          src={file.preview}
          alt={file.name}
          className="h-10 w-10 rounded object-cover"
        />
      )
    }
    if (isImageFile(file)) return <ImageIcon className="h-5 w-5 text-primary" />
    if (file.type.includes("pdf")) return <FileText className="h-5 w-5 text-destructive" />
    return <File className="h-5 w-5 text-muted-foreground" />
  }

  return (
    <div className={cn("space-y-4", className)}>
      <div
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={cn(
          "relative flex flex-col items-center justify-center rounded-lg border-2 border-dashed p-8 transition-colors",
          isDragOver
            ? "border-primary bg-primary/5"
            : "border-muted-foreground/25 hover:border-muted-foreground/50",
          disabled && "pointer-events-none opacity-50",
        )}
      >
        <input
          ref={inputRef}
          type="file"
          multiple
          accept={accept}
          aria-label={dropzoneText}
          onChange={handleInputChange}
          className="absolute inset-0 cursor-pointer opacity-0"
          disabled={disabled}
        />
        <Upload className="mb-2 h-8 w-8 text-muted-foreground" aria-hidden="true" />
        <p className="mb-1 text-sm text-muted-foreground">{dropzoneText}</p>
        <p className="text-xs text-muted-foreground">
          {accept
            ? `Accepted: ${accept}`
            : "Supports images, PDFs, and documents"}
        </p>
      </div>

      {error && (
        <div role="alert" className="flex items-center gap-2 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{error}</span>
          <Button
            variant="ghost"
            size="icon"
            className="ml-auto h-5 w-5"
            onClick={() => setError(null)}
            aria-label="Dismiss error"
          >
            <X className="h-3 w-3" aria-hidden="true" />
          </Button>
        </div>
      )}

      {files.length > 0 && (
        <div className="space-y-2">
          {files.map((file) => (
            <div
              key={file.id}
              className="flex items-center gap-3 rounded-lg border bg-card p-3"
            >
              {getFileIcon(file)}
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{file.name}</p>
                <p className="text-xs text-muted-foreground">
                  {formatFileSize(file.size)}
                </p>
                {uploadProgress[file.id] !== undefined && (
                  <Progress
                    value={uploadProgress[file.id]}
                    className="mt-1 h-1.5"
                  />
                )}
              </div>
              {uploadProgress[file.id] === 100 && (
                <CheckCircle2 className="h-4 w-4 text-emerald-500" aria-label="Upload complete" />
              )}
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 shrink-0"
                onClick={() => removeFile(file.id)}
                disabled={disabled}
                aria-label={`Remove ${file.name}`}
              >
                <X className="h-3.5 w-3.5" aria-hidden="true" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
