<script>
    let userName = '';
    let dateOfBirth = '';
    let email = '';
    let phoneNumber = '';
    let education = 'Elementary School';
    let isMarried = false;
    let numberOfKids = 0;
    let isSubmitting = false;
    let submitMessage = '';

    const educationOptions = [
        'Elementary School',
        'Middle School', 
        'High School',
        'College',
        'Graduate'
    ];

    async function handleSubmit() {
        isSubmitting = true;
        submitMessage = '';
        
        const formData = {
            userName,
            dateOfBirth,
            email,
            phoneNumber,
            education,
            isMarried,
            numberOfKids
        };

        try {
            const response = await fetch('/api/v1/test-form-clicked', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(formData)
            });

            if (response.ok) {
                submitMessage = 'Form submitted successfully!';
                // Reset form after successful submission
                userName = '';
                dateOfBirth = '';
                email = '';
                phoneNumber = '';
                education = 'Elementary School';
                isMarried = false;
                numberOfKids = 0;
            } else {
                submitMessage = 'Failed to submit form. Please try again.';
            }
        } catch (error) {
            console.error('Error submitting form:', error);
            submitMessage = 'An error occurred. Please try again.';
        } finally {
            isSubmitting = false;
        }
    }
</script>

<div class="form-container">
    <h1>User Information Form</h1>
    
    <form onsubmit={handleSubmit}> 
        <div class="form-group">
            <label for="userName">User Name:</label>
            <input 
                type="text" 
                id="userName" 
                bind:value={userName}
                required
            />
        </div>
        
        <div class="form-group">
            <label for="dateOfBirth">Date of Birth:</label>
            <input 
                type="date" 
                id="dateOfBirth" 
                bind:value={dateOfBirth}
                required
            />
        </div>
        
        <div class="form-group">
            <label for="email">Email:</label>
            <input 
                type="email" 
                id="email" 
                bind:value={email}
                required
            />
        </div>
        
        <div class="form-group">
            <label for="phoneNumber">Phone Number:</label>
            <input 
                type="tel" 
                id="phoneNumber" 
                bind:value={phoneNumber}
                required
            />
        </div>
        
        <div class="form-group">
            <label for="education">Education:</label>
            <select id="education" bind:value={education}>
                {#each educationOptions as option}
                    <option value={option}>{option}</option>
                {/each}
            </select>
        </div>
        
        <div class="form-group">
            <label for="isMarried">Marriage Status:</label>
            <div class="toggle-container">
                <span class="toggle-label">{isMarried ? 'Married' : 'Single'}</span>
                <div class="toggle-switch" onclick={() => isMarried = !isMarried}>
                    <div class="toggle-slider {isMarried ? 'on' : 'off'}"></div>
                </div>
            </div>
        </div>
        
        <div class="form-group">
            <label for="numberOfKids">Number of Kids: {numberOfKids}</label>
            <input 
                type="range" 
                id="numberOfKids" 
                min="0" 
                max="10" 
                bind:value={numberOfKids}
                class="slider"
            />
        </div>
        
        <div class="form-actions">
            <button 
                type="submit" 
                class="submit-button"
                disabled={isSubmitting}
            >
                {isSubmitting ? 'Submitting...' : 'Submit'}
            </button>
            <button type="button" class="reset-button" onclick={() => {
                userName = '';
                dateOfBirth = '';
                email = '';
                phoneNumber = '';
                education = 'Elementary School';
                isMarried = false;
                numberOfKids = 0;
                submitMessage = '';
            }}>Reset</button>
        </div>
        
        {#if submitMessage}
            <div class="submit-message {submitMessage.includes('successfully') ? 'success' : 'error'}">
                {submitMessage}
            </div>
        {/if}
    </form>
</div>

<style>
    .form-container {
        max-width: 600px;
        margin: 20px auto;
        padding: 20px;
        border: 1px solid #ddd;
        border-radius: 8px;
        box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
        font-family: Arial, sans-serif;
    }
    
    h1 {
        text-align: center;
        color: #333;
        margin-bottom: 30px;
    }
    
    .form-group {
        margin-bottom: 20px;
    }
    
    label {
        display: block;
        margin-bottom: 5px;
        font-weight: bold;
        color: #555;
    }
    
    input, select {
        width: 100%;
        padding: 10px;
        border: 1px solid #ccc;
        border-radius: 4px;
        font-size: 16px;
        box-sizing: border-box;
    }
    
    .toggle-container {
        display: flex;
        align-items: center;
        justify-content: space-between;
    }
    
    .toggle-label {
        font-weight: normal;
    }
    
    .toggle-switch {
        width: 60px;
        height: 30px;
        background-color: #ccc;
        border-radius: 15px;
        position: relative;
        cursor: pointer;
        transition: background-color 0.3s;
    }
    
    .toggle-switch.on {
        background-color: #4CAF50;
    }
    
    .toggle-slider {
        position: absolute;
        top: 2px;
        width: 26px;
        height: 26px;
        border-radius: 50%;
        background-color: white;
        transition: transform 0.3s;
        box-shadow: 0 2px 4px rgba(0,0,0,0.2);
    }
    
    .toggle-slider.off {
        transform: translateX(2px);
    }
    
    .toggle-slider.on {
        transform: translateX(32px);
    }
    
    .slider {
        width: 100%;
        height: 8px;
        margin-top: 10px;
    }
    
    .form-actions {
        display: flex;
        gap: 10px;
        justify-content: center;
        margin-top: 30px;
    }
    
    .submit-button, .reset-button {
        padding: 12px 24px;
        border: none;
        border-radius: 4px;
        font-size: 16px;
        cursor: pointer;
        transition: background-color 0.3s;
    }
    
    .submit-button {
        background-color: #4CAF50;
        color: white;
    }
    
    .submit-button:hover:not(:disabled) {
        background-color: #45a049;
    }
    
    .submit-button:disabled {
        background-color: #cccccc;
        cursor: not-allowed;
    }
    
    .reset-button {
        background-color: #f44336;
        color: white;
    }
    
    .reset-button:hover {
        background-color: #da190b;
    }
    
    .submit-message {
        margin-top: 20px;
        padding: 10px;
        border-radius: 4px;
        text-align: center;
    }
    
    .submit-message.success {
        background-color: #dff0d8;
        color: #3c763d;
        border: 1px solid #d6e9c6;
    }
    
    .submit-message.error {
        background-color: #f2dede;
        color: #a94442;
        border: 1px solid #ebccd1;
    }
</style>
