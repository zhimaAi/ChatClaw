import fileIconCsv from '@/assets/images/file-icons/file-icon-csv.png'
import fileIconDocx from '@/assets/images/file-icons/file-icon-docx.png'
import fileIconHtml from '@/assets/images/file-icons/file-icon-html.png'
import fileIconMd from '@/assets/images/file-icons/file-icon-md.png'
import fileIconNormal from '@/assets/images/file-icons/file-icon-normal.png'
import fileIconOfd from '@/assets/images/file-icons/file-icon-ofd.png'
import fileIconPdf from '@/assets/images/file-icons/file-icon-pdf.png'
import fileIconPpt from '@/assets/images/file-icons/file-icon-ppt.png'
import fileIconTxt from '@/assets/images/file-icons/file-icon-txt.png'
import fileIconXls from '@/assets/images/file-icons/file-icon-xls.png'
import fileIconXlsx from '@/assets/images/file-icons/file-icon-xlsx.png'
import fileIconZip from '@/assets/images/file-icons/file-icon-zip.png'

/**
 * Resolve bundled PNG icon URL for a file extension (no leading dot).
 * Covers knowledge cards, title bar, and chat attachment chips (formerly ChatInputArea FILE_ICON_MAP).
 */
export function getFileTypeIconUrl(ext: string): string {
  const e = String(ext || '')
    .trim()
    .toLowerCase()
  switch (e) {
    case 'pdf':
      return fileIconPdf
    case 'doc':
    case 'docx':
      return fileIconDocx
    case 'ppt':
    case 'pptx':
      return fileIconPpt
    case 'xls':
      return fileIconXls
    case 'xlsx':
      return fileIconXlsx
    case 'txt':
      return fileIconTxt
    case 'md':
    case 'markdown':
      return fileIconMd
    case 'html':
    case 'htm':
      return fileIconHtml
    case 'csv':
      return fileIconCsv
    case 'ofd':
      return fileIconOfd
    case 'json':
    case 'xml':
    case 'rtf':
    case 'log':
      return fileIconTxt
    case 'zip':
    case 'rar':
    case '7z':
    case 'tar':
    case 'gz':
      return fileIconZip
    default:
      return fileIconNormal
  }
}
