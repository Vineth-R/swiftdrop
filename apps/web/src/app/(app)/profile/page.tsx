import type { Metadata } from 'next';

import { ProfileView } from '../../../components/profile/profile-view';

export const metadata: Metadata = { title: 'Profile — SwiftDrop' };

export default function ProfilePage() {
  return <ProfileView />;
}
