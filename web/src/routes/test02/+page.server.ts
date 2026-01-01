import { superValidate } from 'sveltekit-superforms';
import { typebox } from 'sveltekit-superforms/adapters';
import Type from 'typebox';

// Define outside the load function so the adapter can be cached
const schema = Type.Object({
  name: Type.String({ default: 'Hello world!' }),
  email: Type.String({ format: 'email' })
});

export const load = async () => {
  const form = await superValidate(typebox(schema));

  // Always return { form } in load functions
  return { form };
};