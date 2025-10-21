# Registration Page Documentation

This directory contains the components and main page for user registration in the application. Users can register using one of the following methods:

1. **Google Account**: Users can register using their Google account, which simplifies the authentication process.
2. **GitHub Account**: Users can also register using their GitHub account, allowing for easy integration for developers.
3. **Email Registration**: Users can register by providing their email address and creating a password.

## File Descriptions

- **+page.svelte**: The main registration page that imports and displays the registration options for Google, GitHub, and email. It manages the layout and user interface for the registration process.

- **GoogleRegister.svelte**: This component handles the registration process for users who choose to sign up with their Google account. It includes the necessary logic and UI elements for Google authentication.

- **GitHubRegister.svelte**: This component is responsible for the registration process for users signing up with their GitHub account. It includes the necessary logic and UI elements for GitHub authentication.

- **EmailRegister.svelte**: This component provides a form for users to register with their email address. It includes fields for user input and handles the submission of the registration form.

## Setup Instructions

1. Clone the repository to your local machine.
2. Navigate to the project directory.
3. Install the required dependencies using npm:
   ```
   npm install
   ```
4. Run the development server:
   ```
   npm run dev
   ```
5. Open your browser and go to `http://localhost:3000/registration` to access the registration page.

## Additional Notes

Make sure to configure the necessary authentication settings for Google and GitHub in your application to enable these registration methods.