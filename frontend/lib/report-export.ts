import { ensureFileExtension, saveBlobAsFile } from './download'

export type ReportExportFormat = 'markdown' | 'html'

/**
 * Saves a report payload produced by the backend. HTML styling and Markdown
 * rendering intentionally live in the shared backend report package so AI
 * reports use the same visual language as scan-task exports.
 */
export async function saveReportBlob(
  blob: Blob,
  filename: string,
  format: ReportExportFormat,
): Promise<'saved' | 'cancelled'> {
  const baseName = filename.replace(/\.(md|markdown|html?)$/i, '').trim() || 'report'
  return saveBlobAsFile(blob, ensureFileExtension(baseName, format === 'html' ? 'html' : 'md'))
}
