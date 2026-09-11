import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// "Panels" are called "Ticket buttons" in the dashboard; keep old links working.
export const load: PageLoad = ({ params }) => {
	redirect(308, `/servers/${params.id}/buttons${params.rest ? `/${params.rest}` : ''}`);
};
