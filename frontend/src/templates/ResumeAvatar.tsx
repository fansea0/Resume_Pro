import type { ResumeProfile } from '../types/resume';

interface ResumeAvatarProps {
  profile: ResumeProfile;
  className: string;
}

export function ResumeAvatar({ profile, className }: ResumeAvatarProps) {
  if (profile.showAvatar === false) return null;
  const initial = profile.fullName.trim().slice(0, 1) || '简';
  if (profile.avatarUrl) {
    return <img alt={`${profile.fullName || '候选人'}的简历照片`} className={`${className} template-resume-photo`} src={profile.avatarUrl} />;
  }
  return <div aria-label="候选人头像占位" className={className}>{initial}</div>;
}
