/////////////////////////////////////////////////////
// Description:
//  It defines commonly used system table record schemas.
// Created: 2025/11/01 by Chen Ding
/////////////////////////////////////////////////////

import { z} from 'zod';


export const zPromptInfo = z.object({
    prompt_id:          z.number().optional(),
    prompt_name:        z.string().min(1).max(64).regex(/^[a-zA-Z0-9_]+$/),
    prompt_desc:        z.string().max(500).optional(),
    prompt_content:     z.string().min(5),
    prompt_status:      z.enum(['active', 'suspended', 'deleted']),
    prompt_purpose:     z.string().optional(),
    prompt_source:      z.enum(['user entered', 'uploaded', 'crawled']),
    prompt_keywords:    z.string().optional(),
    prompt_tags:        z.string().optional(),
    creator:            z.string().optional(),
    updater:            z.string().optional(),
    creator_loc:        z.string().length(11),
    updater_loc:        z.string().length(11),
    created_at:         z.string().optional(),
    updated_at:         z.string().optional(),
})
