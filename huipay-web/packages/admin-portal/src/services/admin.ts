// 管理后台 API 服务（骨架）
export async function getCurrentAdmin(): Promise<{ id: number; name: string; role: string }> {
  return { id: 1, name: '平台管理员', role: 'admin' };
}

export async function listMerchants(): Promise<unknown[]> {
  return [];
}

export async function listChannels(): Promise<unknown[]> {
  return [];
}

export async function listRiskRules(): Promise<unknown[]> {
  return [];
}