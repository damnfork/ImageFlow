import {
  Image as ImageIcon,
  PlusCircle as PlusCircledIcon,
  X as Cross1Icon,
  MoreHorizontal as DotsHorizontalIcon,
  Trash2 as TrashIcon,
  Clipboard as ClipboardCopyIcon,
  Check as CheckIcon,
  ExternalLink as ExternalLinkIcon,
  Eye as EyeOpenIcon,
  Search as MagnifyingGlassIcon,
  KeyRound as LockClosedIcon,
  LogIn as LoginIcon,
  Unlink as LinkBreak2Icon,
  ArrowRight as ArrowRightIcon,
  ArrowDown as ArrowDownIcon,
  Clock as ClockIcon,
  Tags as TagIcon,
  Plus as PlusIcon,
  Info as InfoCircledIcon,
  Share as Share1Icon,
  ChevronDown as CaretDownIcon,
  Download as DownloadIcon,
  CornerDownRight as EnterIcon,
  Maximize2 as SizeIcon,
  AlertTriangle as ExclamationTriangleIcon,
  Upload as UploadIcon,
  X as Cross2Icon,
  File as FileIcon,
  Settings as GearIcon,
  SlidersHorizontal as MixerHorizontalIcon,
  Calendar as CalendarIcon,
  ClipboardList as ClipboardIcon,
  Copy as CopyIcon,
  Move as TransformIcon,
  RotateCw as ReloadIcon,
  Moon as MoonIcon,
  Sun as SunIcon,
  Menu as HamburgerMenuIcon,
  ChevronRight,
  ChevronLeft,
  ChevronDown,
  ChevronUp,
  MoreVertical as DotsVerticalIcon,
  Mail as EnvelopeClosedIcon,
  User as PersonIcon,
  Heart as HeartIcon,
  Heart as HeartFilledIcon,
  Star as StarIcon,
  Star as StarFilledIcon,
  HelpCircle as QuestionMarkIcon,
  Link as Link1Icon,
  CreditCard as IdCardIcon,
  // 新增图标用于不同格式
  Camera as CameraIcon,
  Sparkles as SparklesIcon,
  Zap as ZapIcon,
  Shield as ShieldIcon,
  Layers as LayersIcon,
  Disc as DiscIcon,
  Hash as HashIcon,
  Code as CodeIcon,
  Archive as ArchiveIcon,
  Package as PackageIcon
} from 'lucide-react';

export {
  ImageIcon, // 图片图标
  PlusCircledIcon, // 添加图标（带圆圈）
  PlusIcon, // 添加图标
  Cross1Icon, // 关闭/删除图标
  Cross2Icon, // 替代关闭图标
  DotsHorizontalIcon, // 更多操作图标
  TrashIcon, // 删除图标
  ClipboardCopyIcon, // 复制图标
  CheckIcon, // 确认/成功图标
  ExternalLinkIcon, // 外部链接图标
  EyeOpenIcon, // 查看图标
  MagnifyingGlassIcon, // 搜索图标
  LockClosedIcon, // 锁定图标
  LoginIcon, // 登录图标
  LinkBreak2Icon, // 链接断开图标
  ArrowRightIcon, // 右箭头图标
  ArrowDownIcon, // 下箭头图标
  ClockIcon, // 时钟/计时图标
  TagIcon, // 标签图标
  InfoCircledIcon, // 信息图标
  Share1Icon, // 分享图标
  CaretDownIcon, // 下拉箭头图标
  DownloadIcon, // 下载图标
  EnterIcon, // 确认/进入图标
  SizeIcon, // 尺寸图标
  ExclamationTriangleIcon, // 警告/错误图标
  UploadIcon, // 上传图标
  FileIcon, // 文件图标
  GearIcon, // 设置图标
  MixerHorizontalIcon, // 过滤/筛选图标
  CalendarIcon, // 日历图标
  ClipboardIcon, // 剪贴板图标
  CopyIcon, // 复制图标
  TransformIcon, // 变换图标
  ReloadIcon, // 重新加载图标
  MoonIcon, // 月亮/夜间模式图标
  SunIcon, // 太阳/日间模式图标
  HamburgerMenuIcon, // 菜单图标
  ChevronRight as ChevronRightIcon, // 右箭头
  ChevronLeft as ChevronLeftIcon, // 左箭头
  ChevronDown as ChevronDownIcon, // 下箭头
  ChevronUp as ChevronUpIcon, // 上箭头
  DotsVerticalIcon, // 垂直更多操作图标
  EnvelopeClosedIcon, // 邮件图标
  PersonIcon, // 人物/用户图标
  HeartIcon, // 心形/喜欢图标
  HeartFilledIcon, // 实心心形图标
  StarIcon, // 星形/收藏图标
  StarFilledIcon, // 实心星形图标
  QuestionMarkIcon, // 问号/帮助图标
  Link1Icon, // 链接图标
  IdCardIcon, // ID卡/身份图标
  // 新增的格式相关图标
  CameraIcon, // 相机图标 - 原始图片
  SparklesIcon, // 闪亮图标 - 优化格式
  ZapIcon, // 闪电图标 - 快速格式
  ShieldIcon, // 盾牌图标 - 安全格式
  LayersIcon, // 层级图标 - 堆叠格式
  DiscIcon, // 圆盘图标 - 存储格式
  HashIcon, // 哈希图标 - 编码格式
  CodeIcon, // 代码图标 - Markdown格式
  ArchiveIcon, // 归档图标 - 压缩格式
  PackageIcon // 包装图标 - 打包格式
};

// 状态图标 - 为不同类型的状态消息提供图标
export const StatusIcon = {
  success: ({ className = "" }: { className?: string }) => (
    <CheckIcon className={`text-green-500 ${className}`} />
  ),
  error: ({ className = "" }: { className?: string }) => (
    <Cross1Icon className={`text-red-500 ${className}`} />
  ),
  warning: ({ className = "" }: { className?: string }) => (
    <ExclamationTriangleIcon className={`text-amber-500 ${className}`} />
  ),
  info: ({ className = "" }: { className?: string }) => (
    <InfoCircledIcon className={`text-blue-500 ${className}`} />
  )
};

// 封装通用 Spinner 组件
export const Spinner = ({ className = "" }: { className?: string }) => (
  <svg 
    className={`animate-spin ${className}`} 
    xmlns="http://www.w3.org/2000/svg" 
    fill="none" 
    viewBox="0 0 24 24"
  >
    <circle 
      className="opacity-25" 
      cx="12" 
      cy="12" 
      r="10" 
      stroke="currentColor" 
      strokeWidth="4" 
    />
    <path 
      className="opacity-75" 
      fill="currentColor" 
      d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
    />
  </svg>
); 